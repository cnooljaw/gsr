package sproto

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	WireVarintName  = "integer" // int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64
	WireBooleanName = "boolean" // bool
	WireBytesName   = "string"  // string, []byte
	WireDoubleName  = "double"
	WireStructName  = "struct" // struct
)

var (
	mutex sync.Mutex
	stMap = make(map[reflect.Type]*SprotoType)
)

type decoder func(inline uint16, hasInline bool, data []byte, st *SprotoField, v reflect.Value, budget *decodeBudget) error

type SprotoField struct {
	Name     string
	OrigName string
	Wire     string
	Tag      int
	Array    bool

	st *SprotoType // for struct types only

	index      []int // index sequence for Value.FieldByIndex
	dec        decoder
	nilable    bool
	unsigned   bool
	pointer    bool
	valueType  reflect.Type
	int64Slice bool

	encOwner            *SprotoType // 绑定 offset 的实际结构体，防止公开 Fields 替换后借用其他布局。
	encOffset           uintptr
	nativeInt64Pointer  bool
	nativeStringPointer bool
}

// parse filed meta information
func (sf *SprotoField) parse(s string) error {
	// children,object,3,array
	fields := strings.Split(s, ",")
	if len(fields) < 2 {
		return fmt.Errorf("sproto: parse(%s) tag must have 2 or more fields", s)
	}
	sf.Wire = fields[0]
	switch sf.Wire {
	case WireVarintName, WireBooleanName, WireBytesName, WireStructName, WireDoubleName:
	default:
		return fmt.Errorf("sproto: parse(%s) unknown wire type: %s", s, sf.Wire)
	}

	var err error
	sf.Tag, err = strconv.Atoi(fields[1])
	if err != nil || sf.Tag < 0 || sf.Tag > 65535 {
		return fmt.Errorf("sproto: invalid field tag %q", s)
	}

	for i := 2; i < len(fields); i++ {
		f := fields[i]
		switch {
		case f == "array":
			sf.Array = true
		case strings.HasPrefix(f, "name="):
			sf.OrigName = f[len("name="):]
		}
	}
	return nil
}

func (sf *SprotoField) assertWire(expectedWire string, expectedArray bool) error {
	if sf.Wire != expectedWire {
		return fmt.Errorf("sproto: field(%s) expect %s but get %s", sf.Name, expectedWire, sf.Wire)
	}
	if sf.Array != expectedArray {
		n := "not"
		if expectedArray {
			n = ""
		}
		return fmt.Errorf("sproto: field(%s) should %s be array", sf.Name, n)
	}
	return nil
}

func (sf *SprotoField) configureType(f *reflect.StructField) error {
	var stype reflect.Type
	var err error
	t1 := f.Type
	if t1.Kind() == reflect.Ptr {
		t1 = t1.Elem()
		if t1.Kind() == reflect.Slice {
			return fmt.Errorf("sproto: pointer to slice is unsupported")
		}
	}

	sf.nilable = f.Type.Kind() == reflect.Ptr || f.Type.Kind() == reflect.Slice
	sf.pointer = f.Type.Kind() == reflect.Ptr
	sf.valueType = t1
	integerType := t1
	if t1.Kind() == reflect.Slice {
		integerType = t1.Elem()
	}
	sf.unsigned = integerType.Kind() >= reflect.Uint && integerType.Kind() <= reflect.Uint64
	switch t1.Kind() {
	case reflect.Bool:
		sf.dec = decodeBool
		err = sf.assertWire(WireBooleanName, false)
	case reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16,
		reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64,
		reflect.Int, reflect.Uint:
		sf.dec = decodeInt
		if t1.Kind() == reflect.Int64 {
			sf.dec = decodeInt64
			if f.Type == reflect.TypeFor[*int64]() {
				sf.dec = decodeInt64Pointer
			}
		}
		err = sf.assertWire(WireVarintName, false)
	case reflect.Float64:
		sf.dec = decodeDouble
		err = sf.assertWire(WireDoubleName, false)
	case reflect.String:
		sf.dec = decodeString
		if f.Type == reflect.TypeFor[*string]() {
			sf.dec = decodeStringPointer
		}
		err = sf.assertWire(WireBytesName, false)
	case reflect.Struct:
		if f.Type.Kind() != reflect.Ptr {
			return fmt.Errorf("sproto: object field must be a pointer")
		}
		stype = t1
		sf.dec = decodeStruct
		err = sf.assertWire(WireStructName, false)
	case reflect.Slice:
		sf.int64Slice = t1 == reflect.TypeFor[[]int64]()
		switch t2 := t1.Elem(); t2.Kind() {
		case reflect.Float64:
			sf.dec = decodeIntSlice
			err = sf.assertWire(WireDoubleName, true)
		case reflect.Bool:
			sf.dec = decodeBoolSlice
			err = sf.assertWire(WireBooleanName, true)
		case reflect.Uint8:
			if sf.Wire == WireBytesName {
				sf.dec = decodeBytes
				err = sf.assertWire(WireBytesName, false)
			} else {
				sf.dec = decodeIntSlice
				err = sf.assertWire(WireVarintName, true)
			}
		case reflect.Int8, reflect.Int16, reflect.Uint16,
			reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64,
			reflect.Int, reflect.Uint:
			sf.dec = decodeIntSlice
			err = sf.assertWire(WireVarintName, true)
		case reflect.String:
			sf.dec = decodeStringSlice
			err = sf.assertWire(WireBytesName, true)
		case reflect.Ptr:
			switch t3 := t2.Elem(); t3.Kind() {
			case reflect.Struct:
				stype = t2.Elem()
				sf.dec = decodeStructSlice
				err = sf.assertWire(WireStructName, true)
			default:
				err = fmt.Errorf("sproto: field(%s) no coders for %s -> %s -> %s", sf.Name, t1.Kind().String(), t2.Kind().String(), t3.Kind().String())
			}
		default:
			err = fmt.Errorf("sproto: field(%s) no coders for %s -> %s", sf.Name, t1.Kind().String(), t2.Kind().String())
		}
	default:
		err = fmt.Errorf("sproto: field(%s) no coders for %s", sf.Name, t1.Kind().String())
	}

	if err != nil {
		return err
	}

	if stype != nil {
		if sf.st, err = getSprotoTypeLocked(stype); err != nil {
			return err
		}
	}
	return nil
}

func (sf *SprotoField) init(f *reflect.StructField) error {
	sf.Name = f.Name
	sf.OrigName = f.Name

	tagString := f.Tag.Get("sproto")
	if tagString == "" {
		sf.Tag = -1
		return nil
	}

	if f.PkgPath != "" {
		return fmt.Errorf("sproto: tagged field must be exported")
	}
	sf.index = f.Index
	sf.encOffset = f.Offset
	sf.nativeInt64Pointer = f.Type == reflect.TypeFor[*int64]()
	sf.nativeStringPointer = f.Type == reflect.TypeFor[*string]()
	if err := sf.parse(tagString); err != nil {
		return err
	}
	if err := sf.configureType(f); err != nil {
		return err
	}
	return nil
}

type SprotoType struct {
	typ                 reflect.Type // 当前结构体的 Go 类型，用于构造独占子对象。
	Name                string       // struct name
	Fields              []*SprotoField
	tagMap              map[int]int // tag -> fileds index
	order               []int       // list of struct field numbers in tag order
	dense               []int       // 低编号稠密字段的 index+1；零表示未知，长度至多 128。
	nativeInt64Pointer  bool
	nativeStringPointer bool // 原始 Go 类型是否包含原生 *string 字段。
	int64Record         bool // 所有声明的协议字段都是原生 *int64，仍需在每次绑定时核对公开描述。
}

func (st *SprotoType) Len() int { return len(st.order) }
func (st *SprotoType) Less(i, j int) bool {
	return st.Fields[st.order[i]].Tag < st.Fields[st.order[j]].Tag
}
func (st *SprotoType) Swap(i, j int) {
	st.order[i], st.order[j] = st.order[j], st.order[i]
}

func (st *SprotoType) FieldByTag(tag int) *SprotoField {
	if tag >= 0 && tag < len(st.dense) {
		if index := st.dense[tag]; index != 0 {
			return st.Fields[index-1]
		}
		return nil
	}
	if index, ok := st.tagMap[tag]; ok {
		return st.Fields[index]
	}
	return nil
}

func GetSprotoType(t reflect.Type) (*SprotoType, error) {
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("sproto: type must have kind struct")
	}
	mutex.Lock()
	if cached, exists := stMap[t]; exists {
		mutex.Unlock()
		return cached, nil
	}
	previous := make(map[reflect.Type]*SprotoType, len(stMap))
	for key, value := range stMap {
		previous[key] = value
	}
	sp, err := getSprotoTypeLocked(t)
	if err != nil {
		stMap = previous
	}
	mutex.Unlock()
	return sp, err
}

func getSprotoTypeLocked(t reflect.Type) (*SprotoType, error) {
	if st, ok := stMap[t]; ok {
		return st, nil
	}

	st := &SprotoType{typ: t, int64Record: true}
	stMap[t] = st

	st.Name = t.Name()
	numField := t.NumField()
	st.Fields = make([]*SprotoField, numField)
	st.order = make([]int, numField)
	st.tagMap = make(map[int]int)

	for i := 0; i < numField; i++ {
		sf := &SprotoField{encOwner: st}
		f := t.Field(i)
		if err := sf.init(&f); err != nil {
			delete(stMap, t)
			return nil, err
		}

		st.nativeInt64Pointer = st.nativeInt64Pointer || sf.nativeInt64Pointer
		st.nativeStringPointer = st.nativeStringPointer || sf.nativeStringPointer
		if sf.Tag >= 0 && !sf.nativeInt64Pointer {
			st.int64Record = false
		}
		st.Fields[i] = sf
		st.order[i] = i
		if sf.Tag >= 0 {
			// check repeated tag
			if _, ok := st.tagMap[sf.Tag]; ok {
				return nil, fmt.Errorf("sproto: field(%s.%s) tag repeated", st.Name, sf.Name)
			}
			st.tagMap[sf.Tag] = i
		}
	}

	// Re-order prop.order
	sort.Sort(st)
	covered, denseLength := 0, 0
	for _, index := range st.order {
		tag := st.Fields[index].Tag
		if tag < 0 {
			continue
		}
		if tag >= 128 {
			break
		}
		covered++
		if tag+1 <= 2*covered {
			denseLength = tag + 1
		}
	}
	if denseLength > 0 {
		st.dense = make([]int, denseLength)
		for tag, index := range st.tagMap {
			if tag < denseLength {
				st.dense[tag] = index + 1
			}
		}
	}
	return st, nil
}

// Get the type and value of a pointer to a struct from interface{}
func getbase(sp interface{}) (t reflect.Type, v reflect.Value, err error) {
	if sp == nil {
		err = ErrNil
		return
	}

	t = reflect.TypeOf(sp)
	if t.Kind() != reflect.Ptr {
		err = ErrNonPtr
		return
	}

	if t.Elem().Kind() != reflect.Struct {
		err = ErrNonStruct
		return
	}

	v = reflect.ValueOf(sp)
	if v.IsNil() {
		err = ErrNil
		return
	}

	return
}
