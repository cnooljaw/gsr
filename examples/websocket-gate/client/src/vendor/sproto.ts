// @ts-nocheck
// Upstream: sealindx/sproto-ts bbee46fd76e2c9a7401ff9efd1dcadb538908130
// BSD-2-Clause; see sproto.LICENSE. Example-only reference implementation.
import { Buffer } from "node:buffer";
// import {Buffer} from "./buffer/buffer";
const gettype=Object.prototype.toString

const SPROTO_TARRAY = 0x80;
const CHUNK_SIZE = 1000;
const SIZEOF_LENGTH = 4;
const SIZEOF_HEADER  = 2;
const SIZEOF_FIELD  = 2;
const SIZEOF_INT64 = 8;
const SIZEOF_INT32 = 4;

const ERROR_TYPE = -3;
const NONZERO_COUNT = new Uint8Array(256);
for (let mask = 1; mask < 256; mask++) NONZERO_COUNT[mask] = NONZERO_COUNT[mask >>> 1] + (mask & 1);
const ZERO_BLOCK = Buffer.alloc(2048);
const EMPTY_UNPACK = Buffer.alloc(0);
const UNPACK_RETAIN_LIMIT = 16 * 1024;

const ENCODE_BUFFERSIZE = 2050;

const REQUEST = 0;
const RESPONSE = 1;

/*
 * gettype.call('aaaa')输出      [object String]
 * gettype.call(2222) 输出      [object Number]
 * gettype.call(true)  输出      [object Boolean]
 * gettype.call(undefined)  输出      [object Undefined]
 * gettype.call(null)  输出   [object Null]
 * gettype.call({})   输出   [object Object]
 * gettype.call([])    输出   [object Array]
 * gettype.call(function(){})     输出   [object Function]
*/
function checktype(value, type) {
	if (type === "array" && Array.isArray(value)) return true;
	if (typeof value === type && type !== "object") return true;
	const description = gettype.call(value);
	if (type === "object" && description === "[object Object]") return true;
	const actual = description.slice(8, -1).toLowerCase();
	return actual === type ? true : actual;
}


function isNull(val) {
	if (val === undefined || val === null) {
		return true;
	}

	return false;
}

function uint64_rshift(num, offset){
    return Math.floor(num / Math.pow(2, offset));
}

function hi_low_int64(low, hi){
    const value = hi * 0x100000000 + (low >>> 0);
    if (!Number.isSafeInteger(value)) throw new Error("Sproto integer exceeds JavaScript safe range");
    return value;
}

function checkInteger(num) {
	if (!Number.isSafeInteger(num)) throw new Error("Sproto integer requires a safe number");
	return num >= -0x80000000 && num <= 0x7fffffff ? SIZEOF_INT32 : SIZEOF_INT64;
}

function fill_size(data, data_idx, sz){
    data[data_idx] = sz & 0xff;
    data[data_idx+1] = (sz >> 8) & 0xff;
    data[data_idx+2] = (sz >> 16) & 0xff;
    data[data_idx+3] = (sz >> 24) & 0xff;
    return sz + SIZEOF_LENGTH;
}


function encode_integer(v, data, data_idx, size) {
    if (size < SIZEOF_LENGTH * 2) {
	return -1;
    }

    data[data_idx+4] = v & 0xff;
    data[data_idx+5] = (v >> 8) & 0xff;
    data[data_idx+6] = (v >> 16) & 0xff;
    data[data_idx+7] = (v >> 24) & 0xff;
    return fill_size(data, data_idx, 4);
}

function encode_uint64(v, data, data_idx, size) {
	if (size < SIZEOF_LENGTH + SIZEOF_INT64) {
		return -1;
	}

	data[data_idx + 4] = v & 0xff;
	data[data_idx + 5] = (v >> 8) & 0xff;
	data[data_idx + 6] = (v >> 16) & 0xff;
	data[data_idx + 7] = (v >> 24) & 0xff;

	let hi =  uint64_rshift(v, 32);
	data[data_idx + 8] = hi & 0xff;
	data[data_idx + 9] = (hi >> 8) & 0xff;
	data[data_idx + 10] = (hi >> 16) & 0xff;
	data[data_idx + 11] = (hi >> 24) & 0xff;
	return fill_size(data, data_idx, SIZEOF_INT64);
}

function decode_uint64(data, data_idx) {
	let result = 0;

	result |= data[data_idx];
	result |= data[(data_idx+1) << 8];
	result |= data[(data_idx+1) << 16];
	result |= data[(data_idx+1) << 24];
	result |= data[(data_idx+1) << 32];
	result |= data[(data_idx+1) << 40];
	result |= data[(data_idx+1) << 48];
	result |= data[(data_idx+1) << 56];
	return result;
}


function checkvalue(value, type, name, index = 0) {
	const actual = checktype(value, type);
	if (actual !== true) {
		console.error("[sproto error]: .%s[%d] is not a %s (is %s)", name, index, type, actual);
		return null;
	}
	return type;
}

function uint32_to_uint64(data, data_idx: number, negative: number|boolean) {
	if (negative === 0 || negative === false) {
		data[data_idx++] = 0;
		data[data_idx++] = 0;
		data[data_idx++] = 0;
		data[data_idx++] = 0;
	} else {
		data[data_idx++] = 0xff;
		data[data_idx++] = 0xff;
		data[data_idx++] = 0xff;
		data[data_idx++] = 0xff;
	}
}


function gen_response(self, response, spindex, session) {
	return function(args) {
		self.header_tmp.type = null;
		self.header_tmp.session = session;
		let size = self.encodeBuffered(self.__pack, self.header_tmp, 0);
		if (size === undefined) throw new TypeError("Sproto encode failed");
		if (response) size = self.encodeBuffered("", args, size, response, spindex);
		if (size === undefined) throw new TypeError("Sproto encode failed");
		return self.pack(self.buffer.subarray(0, size));
	};
}

class Field {
	public elementType: string | null;
	constructor(public name: string, public tag: number, public type: string) {
		this.elementType = type.startsWith("*") ? type.slice(1) : null;
	}
}


class Stype {
	public f: Field[] = [];
	public maxn: number = 0;
	private dense: Field[] = [];
	private byTag = new Map<number, Field>();

	indexFields() {
		let lowCount = 0, length = 0;
		for (const field of this.f) {
			this.byTag.set(field.tag, field);
			if (field.tag >= 0 && field.tag < 128) { lowCount++; length = Math.max(length, field.tag + 1); }
		}
		if (length <= lowCount * 2) {
			this.dense = new Array(length);
			for (const field of this.f) if (field.tag < length) this.dense[field.tag] = field;
		}
	}

	fieldByTag(tag: number) {
		return tag >= 0 && tag < this.dense.length ? this.dense[tag] : this.byTag.get(tag);
	}

	constructor(public name: string) {

	}
}

class Protocol {
	public st: Stype[] = [null, null];
	constructor(public name: string, public tag: number) {

	}
}

class Sproto {
	public t: Stype[] = [];
	public p: Protocol[] = [];
	private buffer: Buffer;
	private integerView: DataView | null = null;
	private unpackBuffer: Buffer | null = null;
	private sz: number;
	private peer: Sproto | null = null;
	public header_tmp;

	constructor(public ctx: string,
				public __session = [],
				public __pack = "package",
				public __pcatch = []) {

		ctx = this.common_filter(ctx);
		this.parse(ctx);
		// this.buffer = Buffer.allocUnsafe(4);
		this.buffer = Buffer.allocUnsafe(2048);
		this.header_tmp = {type: null, session: null};
	}

	private type_create(type, protoname?) {
		let name = type[0];
		if (name.charAt(0) === ".") {
			name = name.substr(1, name.length); // substr 去掉第一个字符的点.
		}

		let stype = new Stype(name);
		let content = type.input.replace(/.?{|}/g, "");
		let lines = content.match(/\w+\s+\-?\d+\s*:\s*\*?\w+/gi);
		let errsyntax = content.match(/[a-z]+\s*:\s*\*?[a-z]+/i)

		if (errsyntax) {
			console.error("[sproto error]: syntax error at tag number:", errsyntax[0]);
		}

		if (isNull(lines)) {
			return stype;
		}

		let maxn = lines.length;
		let offset = 8888888888;
		let offcounter = 0;
		let tags = []

		for (let i of lines) {
			let ft = i.split(/\s*(\-?\d+)?\s*:\s*/g);
			let tag = Number(ft[1]);
			let f = new Field(ft[0], tag, ft[2]);
			stype.f.push(f);

			if (tag < 0) {
				console.error("[sproto error]: syntax error at tag(%d) number less 0 in type %s", tag, protoname?protoname:name);
			}

			if (tags[tag]) {
				console.error("[sproto error]: redefine tag %d in type %s", tag, protoname?protoname:name);
			}

			if (tag - offset > 1) {
				++offcounter;
			}

			offset = tag;
			tags[tag] = true;
		}

		maxn += stype.f[0].tag + offcounter;
		stype.maxn = maxn;
		stype.indexFields();

		return stype;
	}

	protocol_create(protocol: string) {
		let nametag = protocol.match(/\w+\s+\-?\d+/i)[0];
		let arr = nametag.split(/\s+/);
		let name = arr[0];
		let tag = Number(arr[1]);

		if (tag < 0) {
			console.error("[sproto error]: syntax error at tag(%d) number less 0 at protocol %s", tag, name);
		}

		let proto = new Protocol(name, tag);
		this.p[name] = proto;
		this.__pcatch[tag] = proto;

		let requeststr = protocol.match(/request\s*{[^{}]*}/g);
		let responsestr = protocol.match(/response\s*{[^{}]*}/g);
		if (requeststr) {
			let req = ["request"];
			req["input"] = requeststr[0];
			proto.st[REQUEST] = this.type_create(req, name);
		}

		if (responsestr) {
			let resp = ["response"];
			resp["input"] = responsestr[0];
			proto.st[RESPONSE] = this.type_create(resp, name);
		}
	}


	private parse(text: string) {
		let types = text.match(/\.\w+[\s\n]*{[^{}]+}/ig);
		let errsyntax = text.match(/\n\w+[\s\n]*{/ig);

		if (errsyntax) {
			errsyntax.forEach(protoname	=> {
				protoname += " ... }";
			    console.error("[sproto error]: syntax error at proto name:", protoname.replace(/\n/, ""));
			});
		}

		if (isNull(types) === false) {
			for (let i = 0; i < types.length; i++) {
				let mtype = types[i];
				let typestr = mtype.match(/\.\w+/i);
				if (typestr && typestr[0] != null) {
					let stype = this.type_create(typestr);
					this.t[stype.name] = stype;
				}
			}
		}

		let protocols = text.match(/\w+\s+\-?\d+\s*{[\n\t\s]*(request\s*{[^{}]*})?[\n\t\s]*(response\s*{[^{}]*})?[\n\t\s]*}/ig);
		if (isNull(protocols) === false) {
			for (let i = 0; i < protocols.length; ++i) {
				this.protocol_create(protocols[i]);
			}
		}

	}

	// 注释过滤
	private common_filter(text: string) {
		return text.replace(/#.*\n/g, "\n");
	}

	private encode_integer_array(v, args, data, data_idx, size) {
		if (v.length === 0) return 0;
		let width = SIZEOF_INT32;
		for (let index = 0; index < v.length; index++) {
			if (checkvalue(v[index], "number", args.name, index) === null) return ERROR_TYPE;
			if (checkInteger(v[index]) === SIZEOF_INT64) width = SIZEOF_INT64;
		}
		const length = 1 + width * v.length;
		if (length > size) return -1;
		data[data_idx] = width;
		let offset = data_idx + 1;
		// data 始终是实例编码缓冲；扩容时失效视图，保留实际 byteOffset 和有界长度。
		const view = this.integerView ??= new DataView(data.buffer, data.byteOffset, data.byteLength);
		if (width === SIZEOF_INT32) {
			for (let index = 0; index < v.length; index++, offset += SIZEOF_INT32) {
				view.setUint32(offset, v[index] >>> 0, true);
			}
		} else {
			for (let index = 0; index < v.length; index++, offset += SIZEOF_INT64) {
				const value = v[index];
				view.setUint32(offset, value >>> 0, true);
				view.setInt32(offset + SIZEOF_INT32, Math.floor(value / 0x100000000), true);
			}
		}
		return length;
	}

	private encode_string_array(values, field, output, offset, available) {
		const start = offset;
		for (let index = 0; index < values.length; index++) {
			if (checkvalue(values[index], "string", field.name, index) === null) return ERROR_TYPE;
			const text = typeof values[index] === "string" ? values[index] : String(values[index]);
			const length = Buffer.byteLength(text);
			if (length + SIZEOF_LENGTH > available) return -1;
			fill_size(output, offset, length);
			output.write(text, offset + SIZEOF_LENGTH, length, "utf8");
			offset += SIZEOF_LENGTH + length;
			available -= SIZEOF_LENGTH + length;
		}
		return offset - start;
	}

	private encode_boolean_array(values, field, output, offset, available) {
		if (values.length > available) return -1;
		for (let index = 0; index < values.length; index++) {
			if (checkvalue(values[index], "boolean", field.name, index) === null) return ERROR_TYPE;
			output[offset + index] = values[index] ? 1 : 0;
		}
		return values.length;
	}

	private encode_object_array(values, field, output, offset, available, spindex) {
		const start = offset;
		const type = this.vquerytype(field.elementType, spindex);
		for (let index = 0; index < values.length; index++) {
			if (available < SIZEOF_LENGTH) return -1;
			const length = this.lencode(field.elementType, values[index], offset + SIZEOF_LENGTH, type, spindex);
			if (length < 0) return length;
			if (length + SIZEOF_LENGTH > available) return -1;
			fill_size(output, offset, length);
			offset += SIZEOF_LENGTH + length;
			available -= SIZEOF_LENGTH + length;
		}
		return offset - start;
	}

	private encode_array(values, field, output, offset, available, spindex) {
		available -= SIZEOF_LENGTH;
		if (available < 0) return -1;
		let length;
		switch (field.elementType) {
		case "integer":
			length = this.encode_integer_array(values, field, output, offset + SIZEOF_LENGTH, available);
			break;
		case "string":
			length = this.encode_string_array(values, field, output, offset + SIZEOF_LENGTH, available);
			break;
		case "boolean":
			length = this.encode_boolean_array(values, field, output, offset + SIZEOF_LENGTH, available);
			break;
		default:
			length = this.encode_object_array(values, field, output, offset + SIZEOF_LENGTH, available, spindex);
		}
		return length < 0 ? length : fill_size(output, offset, length);
	}

	encode(typeName: string, message: any, st?, spindex?) {
		let size;
		while ((size = this.lencode(typeName, message, 0, st, spindex)) < 0) {
			if (size === ERROR_TYPE) return;
			const capacity = this.buffer.length * 2;
			if (capacity > 8 * 1024 * 1024) throw new Error("Sproto encode size limit");
			this.buffer = Buffer.allocUnsafe(capacity);
			this.integerView = null;
		}
		if (size > 8 * 1024 * 1024) throw new Error("Sproto encode size limit");
		const result = Buffer.allocUnsafe(size);
		this.buffer.copy(result, 0, 0, size);
		return result;
	}

	// scratch 只供本次同步编码借用，只有 Encode 的副本或 Pack 的独占输出可以返回调用方。
	private encodeBuffered(typeName: string, message: any, start: number, st?, spindex?) {
		const maxSize = 8 * 1024 * 1024;
		for (;;) {
			const size = this.lencode(typeName, message, start, st, spindex);
			if (size === ERROR_TYPE) return;
			if (size > maxSize) throw new Error("Sproto encode size limit");
			if (size >= 0) return start + size;
			const capacity = Math.min(this.buffer.length * 2, maxSize + start);
			if (capacity <= this.buffer.length) throw new Error("Sproto encode size limit");
			const expanded = Buffer.allocUnsafe(capacity);
			// body 扩容时只保留已完成的 package；当前未完成的 body 会重新编码。
			if (start > 0) this.buffer.copy(expanded, 0, 0, start);
			this.buffer = expanded;
			this.integerView = null;
		}
	}

	private lencode(typename: string, message: any, startpoint: number, st?, spindex?) {
		const type = st || this.vquerytype(typename, spindex);
		if (type === undefined) return ERROR_TYPE;
		const bodyStart = startpoint + SIZEOF_HEADER + type.maxn * SIZEOF_FIELD;
		if (bodyStart > this.buffer.length) return -1;
		let fieldOffset = startpoint + SIZEOF_HEADER;
		let bodyOffset = bodyStart;
		let count = 0;
		let previousTag = -1;
		for (const field of type.f) {
			const value = message[field.name];
			if (isNull(value)) continue;
			let header = 0;
			let length = 0;
			const available = this.buffer.length - bodyOffset;
			if (field.elementType !== null) {
				if (checkvalue(value, "array", field.name) === null) return ERROR_TYPE;
				length = this.encode_array(value, field, this.buffer, bodyOffset, available, spindex);
			} else {
				switch (field.type) {
				case "boolean":
					if (checkvalue(value, "boolean", field.name) === null) return ERROR_TYPE;
					header = (Number(value) + 1) * 2;
					break;
				case "integer":
					if (checkvalue(value, "number", field.name) === null) return ERROR_TYPE;
					const width = checkInteger(value);
					if (value >= 0 && value < 0x7fff) header = (value + 1) * 2;
					else if (width === SIZEOF_INT32) length = encode_integer(value, this.buffer, bodyOffset, available);
					else length = encode_uint64(value, this.buffer, bodyOffset, available);
					break;
				case "string":
					if (checkvalue(value, "string", field.name) === null) return ERROR_TYPE;
					const text = typeof value === "string" ? value : String(value);
					const bytes = Buffer.byteLength(text);
					if (bytes + SIZEOF_LENGTH > available) return -1;
					fill_size(this.buffer, bodyOffset, bytes);
					this.buffer.write(text, bodyOffset + SIZEOF_LENGTH, bytes, "utf8");
					length = SIZEOF_LENGTH + bytes;
					break;
				default:
					if (checkvalue(value, "object", field.name) === null) return ERROR_TYPE;
					if (available < SIZEOF_LENGTH) return -1;
					const nested = this.lencode(field.type, value, bodyOffset + SIZEOF_LENGTH, null, spindex);
					length = nested < 0 ? nested : fill_size(this.buffer, bodyOffset, nested);
				}
			}
			if (length < 0) return length;
			bodyOffset += length;
			const gap = field.tag - previousTag - 1;
			if (gap > 0) {
				const skip = (gap - 1) * 2 + 1;
				if (skip > 0xffff) return -1;
				this.buffer.writeUInt16LE(skip, fieldOffset);
				fieldOffset += SIZEOF_FIELD;
				count++;
			}
			this.buffer.writeUInt16LE(header, fieldOffset);
			fieldOffset += SIZEOF_FIELD;
			count++;
			previousTag = field.tag;
		}
		this.buffer.writeUInt16LE(count, startpoint);
		const bodyLength = bodyOffset - bodyStart;
		// copy 对同一 Buffer 的重叠区间使用安全搬移，不再逐字节压紧 header 后的数据。
		if (fieldOffset !== bodyStart) this.buffer.copy(this.buffer, fieldOffset, bodyStart, bodyOffset);
		return SIZEOF_HEADER + count * SIZEOF_FIELD + bodyLength;
	}

	decode_array(type: string, buffer, data_idx: number, sz: number, st?, reqdecode?) {
		let result :any= [];

		switch (type) {
		case "integer":
			let len = buffer[data_idx++];
			--sz;
			if (len === SIZEOF_INT32) {
				if (sz % SIZEOF_INT32 != 0) {
					return false;
				}

				for (let i = 0; i < sz/SIZEOF_INT32; ++i) {
					result[i] = this.todword(buffer, data_idx);
					data_idx += SIZEOF_INT32;
				}
			} else if (len === SIZEOF_INT64) {
				if (sz % SIZEOF_INT64 !== 0) {
					return false;
				}

				for (let i = 0; i < sz/SIZEOF_INT64; ++i) {
					let	low = this.todword(buffer, data_idx, true);
					let	hi = this.todword(buffer, data_idx + SIZEOF_INT32);

					result[i] = hi_low_int64(low, hi);
					data_idx += SIZEOF_INT64;
				}
			} else {
				result = false;
			}
			break;
		case "string":
			let i = 0;
			for (;;) {
				sz -= SIZEOF_LENGTH;
				let len = this.todword(buffer, data_idx);
				data_idx += SIZEOF_LENGTH;
				sz -= len;

				result[i++] = buffer.toString("utf8", data_idx, data_idx + len);

				data_idx += len;
				if (sz == 0) {
					break;
				} else if (sz < 0) {
					return false;
				}
			}
			break;
		case "boolean":
			for (let i = 0; i < sz; ++i) {
				result[i] = buffer[data_idx + i] === 0 ? false : true;
			}
			break;
		default:
			if (sz === 0) {
				return result;
			}else if (sz < 0) {
				return false;
			}

			let j = 0;
			for (;;) {
				sz -= SIZEOF_LENGTH;
				let len = this.todword(buffer, data_idx);
				data_idx += SIZEOF_LENGTH;
				result[j] = {};

				this.ldecode(type, buffer, data_idx, result[j++], null, reqdecode);
				data_idx += len;
				sz -= len;

				if (sz == 0) {
					break;
				} else if (sz < 0) {
					return false;
				}
			}

			break;
		}

		return result;
	}

	decode(typename: string, buffer: Buffer) {
		let result = {};
		let sz = this.ldecode(typename, buffer, 0, result);
		if (sz < 0) {
			console.error("[sproto error]: decode failed");
			return;
		}
		return result;
	}

	private ldecode(typename: string, buffer: Buffer, startpoint:number, result: object, st?, reqdecode?) {
		const type = st || this.vquerytype(typename, reqdecode);
		let fn = this.toword(buffer, startpoint);
		let size = SIZEOF_HEADER;

		let field_idx = startpoint + SIZEOF_HEADER;
		let data_idx = startpoint + SIZEOF_HEADER + fn * SIZEOF_FIELD;
		let tag = -1;

		for (var i = 0; i < fn; ++i) {
			++tag;

			let sz = 0;
			let value = this.toword(buffer, field_idx);
			field_idx += SIZEOF_FIELD;
			size += SIZEOF_FIELD;

			if ((value & 1) !== 0) {
				tag += (value >> 1);
				continue;
			}

			value = (value>>1) - 1;
			let currentdata_idx = data_idx;
			// 无论 schema 是否认识字段，都先消费它的 body，保持后续字段位置正确。
			if (value < 0) {
				if (currentdata_idx + SIZEOF_LENGTH > buffer.length) return -1;
				sz = this.todword(buffer, currentdata_idx);
				currentdata_idx += SIZEOF_LENGTH;
				if (sz < 0 || sz > buffer.length - currentdata_idx) return -1;
				data_idx = currentdata_idx + sz;
				size += SIZEOF_LENGTH + sz;
			}
			const f = type.fieldByTag(tag);
			if (f === undefined) continue;
			if (value < 0) {
				if (f.elementType !== null) {
					let array_res :any = [];
					if (sz > 0) {
						let t = f.elementType;
						array_res = this.decode_array(t, buffer, currentdata_idx, sz, st, reqdecode);
						if (array_res === false) {
							console.error("[sproto error]: decode array filed(%s) failed", f.name);
							return -1;
						}
					}
					result[f.name] = array_res;
				} else {
					switch (f.type) {
					case "string":
						result[f.name] = buffer.toString("utf8", currentdata_idx, currentdata_idx + sz);
						break;
					case "integer":
						let low = this.todword(buffer, currentdata_idx);
						let hi = 0;
						if (sz === SIZEOF_INT64) {
							low = this.todword(buffer, currentdata_idx, true);
							hi = this.todword(buffer, currentdata_idx + SIZEOF_INT32);
						} else if (sz !== SIZEOF_INT32) {
							return -1;
						}

						result[f.name] = sz === SIZEOF_INT32 ? low : hi_low_int64(low, hi);
						break;
					default:
						let subres = {};
						let tmpsz = this.ldecode(f.type, buffer, currentdata_idx, subres, null, reqdecode);
						if (tmpsz < 0) {
							return tmpsz;
						}
						size += tmpsz;
						result[f.name] = subres;
						break;
					}
				}
			} else if (f.type !== "integer" && f.type !== "boolean"){ // value>=0，必须是 integer 或者 boolean，所以要先判断 f.type类型
				console.error("[sproto error]: field(%s) type:", f.name, f.type);
				return -1;
			} else {	// value >= 0
				if (f.type === "boolean") {
					result[f.name] = value !== 0 ? true : false;
				} else {
					result[f.name] = value;
				}
			}
		}

		return size;
	}

	pencode(typename: string, tbl: object) {
		const size = this.encodeBuffered(typename, tbl, 0);
		if (size === undefined) throw new TypeError("Sproto encode failed");
		return this.pack(this.buffer.subarray(0, size));
	}

	pdecode(typename: string, buffer) {
		return this.decode(typename, this.unpackBuffered(buffer));
	}

	pack_seg(source, sourceIndex, output, outputIndex) {
		let count = 0, mask = 0;
		const value0 = source[sourceIndex + 0];
		if (value0 !== 0) { output[outputIndex + ++count] = value0; mask |= 1; }
		const value1 = source[sourceIndex + 1];
		if (value1 !== 0) { output[outputIndex + ++count] = value1; mask |= 2; }
		const value2 = source[sourceIndex + 2];
		if (value2 !== 0) { output[outputIndex + ++count] = value2; mask |= 4; }
		const value3 = source[sourceIndex + 3];
		if (value3 !== 0) { output[outputIndex + ++count] = value3; mask |= 8; }
		const value4 = source[sourceIndex + 4];
		if (value4 !== 0) { output[outputIndex + ++count] = value4; mask |= 16; }
		const value5 = source[sourceIndex + 5];
		if (value5 !== 0) { output[outputIndex + ++count] = value5; mask |= 32; }
		const value6 = source[sourceIndex + 6];
		if (value6 !== 0) { output[outputIndex + ++count] = value6; mask |= 64; }
		const value7 = source[sourceIndex + 7];
		if (value7 !== 0) { output[outputIndex + ++count] = value7; mask |= 128; }
		if (count === 8) return 10;
		output[outputIndex] = mask;
		return count + 1;
	}

	write_ff(source, sourceStart, output, destination, length) {
		const aligned = (length + 7) & ~7;
		const available = Math.min(length, source.length - sourceStart);
		output[destination] = 0xff;
		output[destination + 1] = aligned / 8 - 1;
		source.copy(output, destination + 2, sourceStart, sourceStart + available);
		if (available < aligned) output.fill(0, destination + 2 + available, destination + 2 + aligned);
	}

	// 最后不足八字节的 word 隐含补零，不为最多七个字节另分配临时 Buffer。
	private packTail(source, start, output, destination) {
		let count = 0, mask = 0;
		for (let index = 0; index < source.length - start; index++) {
			const value = source[start + index];
			if (value !== 0) { output[destination + ++count] = value; mask |= 1 << index; }
		}
		output[destination] = mask;
		return count + 1;
	}

	pack(buffer:Buffer) {
		const srcsz = buffer.length;
		let dstsz = srcsz + Math.ceil(srcsz / 2048) * 2 + 8;

		let dstbuffer = Buffer.allocUnsafe(dstsz);
		let dstidx = 0;
		let srcidx;
		let ff_n = 0;
		let ff_srcstart = -1;
		let ff_desstart = -1;
		let ff_buffer;

		for (let i = 0; i < srcsz; i += 8) {
			srcidx = i;

			let n = 0;
			n = i + 8 <= srcsz
				? this.pack_seg(buffer, srcidx, dstbuffer, dstidx)
				: this.packTail(buffer, srcidx, dstbuffer, dstidx);
			// FF 合并规则集中在段状态所在处，逐 word 写出只负责 mask 和非零字节。
			if (ff_n > 0 && n >= 7) n = 8;
			dstsz -= n;
			// 普通混合 word 不参与 FF 收尾和长零段扫描。
			if (n < 10 && n > 1 && ff_n === 0) {
				dstidx += n;
				continue;
			}

			if (n === 10) {
				ff_srcstart = srcidx;
				ff_desstart = dstidx;
				ff_buffer = buffer;
				ff_n = 1;
				if (srcsz - i >= 64) {
					const words = this.nonzeroWords(buffer, srcidx + 8, srcsz - i - 8);
					ff_n += words;
					i += words * 8;
					n += words * 8;
					dstsz -= words * 8;
					if (ff_n === 256) {
						if (dstsz >= 0) this.write_ff(ff_buffer, ff_srcstart, dstbuffer, ff_desstart, 2048);
						ff_n = 0;
					}
				}
			} else if (n === 8 && ff_n > 0) {
				++ff_n;
				if (ff_n === 256) {
					if (dstsz >= 0) {
						this.write_ff(ff_buffer, ff_srcstart, dstbuffer, ff_desstart, 256*8);
					}
					ff_n = 0;
				}
			} else {
				if (ff_n > 0) {
					if (dstsz >= 0) {
						this.write_ff(ff_buffer, ff_srcstart, dstbuffer, ff_desstart, ff_n*8);
					}
					ff_n = 0;
				}
				if (n === 1 && srcsz - i >= 1024) {
					const words = this.zeroWords(buffer, srcidx + 8, srcsz - i - 8);
					if (words > 0) {
						dstbuffer.fill(0, dstidx, dstidx + 1 + words);
						i += words * 8;
						n += words;
						dstsz -= words;
					}
				}
			}

			dstidx += n;
		}

		if(dstsz >= 0){
			if(ff_n === 1) {
				this.write_ff(ff_buffer, ff_srcstart, dstbuffer, ff_desstart, 8);
			}
			else if (ff_n > 1) {
				this.write_ff(ff_buffer, ff_srcstart, dstbuffer, ff_desstart, srcsz - ff_srcstart);
			}
		}

		return dstbuffer.slice(0, dstidx);
	}

	// 只在长连续块确认后使用原生有界扫描，普通数值消息沿用逐 word 路径。
	private nonzeroWords(source, start, available) {
		const bytes = Math.min(2040, available & ~7);
		const zero = source.subarray(start, start + bytes).indexOf(0);
		return zero < 0 ? bytes >>> 3 : zero >>> 3;
	}

	private zeroWords(source, start, available) {
		if (source.readUInt32LE(start) !== 0 || source.readUInt32LE(start + 4) !== 0) return 0;
		const bytes = Math.min(2048, available & ~7);
		return source.compare(ZERO_BLOCK, 0, bytes, start, start + bytes) === 0 ? bytes >>> 3 : 0;
	}

	unpack(buffer: Buffer) {
		const maxSize = 8 * 1024 * 1024;
		// 长 FF 块使尺寸扫描可以整块跳过，避免大段原始字节分配双倍空间。
		// 混合数值包保留单遍路径，计数沿用 lunpack 的完整边界和预算检查。
		const capacity = buffer.length >= 2048 && this.preferExactUnpack(buffer)
			? this.lunpack(buffer, EMPTY_UNPACK) : Math.min(maxSize, Math.max(8, buffer.length * 2));
		if (capacity < 0) return null;
		let outbuffer = Buffer.allocUnsafe(capacity);
		const size = this.lunpack(buffer, outbuffer);
		if (size < 0) {
			return null;
		}
		if (size > capacity) {
			// 首次扫描计算完整输出长度；按确切大小分配，再写入完整内容。
			outbuffer = Buffer.allocUnsafe(size);
			if (this.lunpack(buffer, outbuffer) !== size) {
				return null;
			}
		}
		return outbuffer.subarray(0, size);
	}

	// 最多看八个 packed 段；FF 字节已占输入的四分之三才值得完整预扫描。
	private preferExactUnpack(buffer: Buffer) {
		let offset = 0, literalBytes = 0;
		for (let word = 0; word < 8 && offset < buffer.length; word++) {
			const mask = buffer[offset++];
			if (mask !== 255) {
				offset += NONZERO_COUNT[mask];
				continue;
			}
			if (offset >= buffer.length) return false;
			const length = (buffer[offset++] + 1) * 8;
			if (length > buffer.length - offset) return false;
			literalBytes += length;
			if (literalBytes * 4 >= buffer.length * 3) return true;
			offset += length;
		}
		return false;
	}

	// 只在同步解码期间借用；ldecode 产出的数字、字符串和数组均不保留字节视图。
	private unpackBuffered(buffer: Buffer) {
		if (buffer.length > UNPACK_RETAIN_LIMIT) return this.unpack(buffer);
		let output = this.unpackBuffer ?? Buffer.allocUnsafe(2048);
		const size = this.lunpack(buffer, output);
		if (size < 0) return null;
		if (size > output.length) {
			const capacity = size <= UNPACK_RETAIN_LIMIT
				? Math.min(UNPACK_RETAIN_LIMIT, Math.max(size, output.length * 2)) : size;
			output = Buffer.allocUnsafe(capacity);
			if (this.lunpack(buffer, output) !== size) return null;
		}
		// 偶发大包仅使用调用内临时缓冲，不抬高每个连接长期保留的容量。
		if (output.length <= UNPACK_RETAIN_LIMIT) this.unpackBuffer = output;
		return output.subarray(0, size);
	}

	lunpack(source: Buffer, output: Buffer) {
		let sourceOffset = 0;
		let destination = 0;
		while (sourceOffset < source.length) {
			let mask = source[sourceOffset++];
			if (mask === 0 && sourceOffset < source.length && source[sourceOffset] === 0) {
				const start = destination;
				destination += 8;
				// 凑齐四个零头再展开计数，仍逐字节处理剩余尾部。
				if (sourceOffset + 3 <= source.length && source[sourceOffset + 1] === 0 && source[sourceOffset + 2] === 0) {
					sourceOffset += 3;
					destination += 24;
					while (destination <= 8 * 1024 * 1024 && sourceOffset + 4 <= source.length
						&& (source[sourceOffset] | source[sourceOffset + 1] | source[sourceOffset + 2] | source[sourceOffset + 3]) === 0) {
						sourceOffset += 4;
						destination += 32;
					}
				}
				while (destination <= 8 * 1024 * 1024 && sourceOffset < source.length && source[sourceOffset] === 0) {
					sourceOffset++;
					destination += 8;
				}
				if (destination > 8 * 1024 * 1024) throw new Error("Sproto unpack size limit");
				if (start < output.length) output.fill(0, start, Math.min(destination, output.length));
				continue;
			}
			if (mask === 255) {
				if (sourceOffset >= source.length) return -1;
				const length = (source[sourceOffset++] + 1) * 8;
				if (length > source.length - sourceOffset) return -1;
				if (length > 8 * 1024 * 1024 - destination) throw new Error("Sproto unpack size limit");
				if (length <= output.length - destination) source.copy(output, destination, sourceOffset, sourceOffset + length);
				sourceOffset += length;
				destination += length;
				continue;
			}
			if (NONZERO_COUNT[mask] > source.length - sourceOffset) return -1;
			if (8 > 8 * 1024 * 1024 - destination) throw new Error("Sproto unpack size limit");
			if (destination >= output.length) {
				// 首次缓冲耗尽后只验证和计数，不再逐字节做无效写入判断。
				sourceOffset += NONZERO_COUNT[mask];
				destination += 8;
				continue;
			}
			if (destination + 8 <= output.length) {
				output[destination + 0] = (mask & 1) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 1] = (mask & 2) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 2] = (mask & 4) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 3] = (mask & 8) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 4] = (mask & 16) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 5] = (mask & 32) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 6] = (mask & 64) !== 0 ? source[sourceOffset++] : 0;
				output[destination + 7] = (mask & 128) !== 0 ? source[sourceOffset++] : 0;
			} else {
				// 保留缓冲不足时的完整计数与前缀写入语义。
				for (let index = 0; index < 8; index++) {
					const value = (mask & 1) !== 0 ? source[sourceOffset++] : 0;
					if (destination + index < output.length) output[destination + index] = value;
					mask >>>= 1;
				}
			}
			destination += 8;
		}
		return destination;
	}

	private toword(p, pos) {
		return p[pos] | p[pos + 1]<<8;
	}

	private todword(p, pos, uint = false) {
		let num = p[pos] | p[pos + 1] << 8 | p[pos + 2] << 16 | p[pos + 3] << 24;
		if (uint) {
			num = num >>> 0;
		}

		return num;
	}

	private querytype(name: string) {
		return this.t[name];
	}

	private vquerytype(name: string, reqdecode?) {
		if (isNull(reqdecode)) {
			return this.t[name];
		} else {
			let sp = this.peer || this;
			return sp.t[name];
		}
	}

	private queryprotocol(name: string) {
		return this.p[name];
	}


	host(sp: Sproto) {
		this.peer = sp;
		this.__pcatch = null;
	}

	attach() {
		return function (name, args, session?) {
			let p = this.queryprotocol(name);
			if (isNull(p)) {
				console.error("[sproto error]: can't found ", name);
				return ;
			}
			this.header_tmp.type = p.tag;
			this.header_tmp.session = session;

			let size = this.encodeBuffered(this.__pack, this.header_tmp, 0);
			if (size === undefined) throw new TypeError("Sproto encode failed");

			if (session) {
				this.__session[session] = p.st[RESPONSE] || true;
			}

			if (p.st[REQUEST]) {
				size = this.encodeBuffered(p.st[REQUEST].name, args, size, p.st[REQUEST]);
			}
			if (size === undefined) throw new TypeError("Sproto encode failed");
			return this.pack(this.buffer.subarray(0, size));
		}.bind(this);
	}

	dispatch(buffer: Buffer, spindex: number = 0) {
		let bin = this.unpackBuffered(buffer);
		this.header_tmp.type = null;
		this.header_tmp.session = null;

		let header = this.header_tmp;
		let sz = this.ldecode(this.__pack, bin, 0, this.header_tmp);
		if (sz < 0) {
			console.error("[sproto error]: decode failed");
			return;
		}
		if (!isNull(header.type)) {
			// request
			let result = {};
			let p = null;
			if (this.__pcatch === null) {
				p = (this.peer || this).__pcatch[header.type];
			} else {
				p = this.__pcatch[header.type];
			}

			if (p && p.st[REQUEST]) {
				let err = this.ldecode(p.st[REQUEST].name, bin, sz, result, p.st[REQUEST], spindex);
				if (err < 0) {
					console.error("[sproto error]: decode failed");
					return;
				}
			} else {
				console.error("[sproto error]: can't find protocol by tag:", header.type);
			}

			let session = this.header_tmp.session;
			if (session) {
				return {replay: "REQUEST", name: p.name, result: result, response: gen_response(this, p.st[RESPONSE], spindex, session).bind(this)};
			} else {
				return {replay: "REQUEST", name: p.name, result: result, response: null};
			}

		} else {
			// response
			let session = this.header_tmp.session;
			if (isNull(session)) {
				console.error("[sproto error]: session not found");
				return;
			}

			let response = this.__session[session];
			if (isNull(response)) {
				console.error("[sproto error]: Unknown session", session);
				return;
			}

			delete this.__session[session];
			if (response === true) {
				return {replay: "RESPONSE", session: session, result: null};
			} else {
				let result = {};
				let err  = this.ldecode(response.name, bin, sz, result, response);
				if (err < 0) {
					console.error("[sproto error]: decode failed");
					return;
				}
				return {replay: "RESPONSE", session: session, result: result};
			}
		}
	}
}

export { Sproto };
