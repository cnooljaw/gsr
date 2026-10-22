# 整数记录写出：独立 header 输出游标

基线gosproto `9048cfa`，完整Git归档。本轮不合并局部预算/最终布局追加/Decode等候选。

## 两次编译预检

第一版在可信的header区域内使用`headers[:2:2]`写出并推进游标，符号188→208条、panic调用点14→16。固定前缀主要证明cap，而后续游标前移仍需len信息，未达到减少检查目标。仅编译与契约/分配预检，未产生任何计时样本；initial-preflight保留全部命令、plan、源和汇编，initial-prepare.py.txt保留复现脚本。

第二版直接调用`writeUint16(headers,...)`再推进，复用其已有的二字节len检查。符号188→200条、panic调用点14→13。它减少检查点，却增加游标状态更新，不能凭静态数量预先判定更快。只有这版进入固定计时矩阵，未按任何已测速度择优选版本。

## 候选边界

只有encodeIntegerRecordInto的header定位变化：写入record count、排除nil之后，从独占output取出`output[2:2+count*2:2+count*2]`；每次skip/inline/外置零header写同样两个字节，然后前移两字节。bodyOffset仍独立维护，内容写出/长度和返回消费大小不变。

同次预检已证明count与当前记录header数，header区域不含内容。空记录/全缺失字段取得空视图但不会写入；nil仍直接返回2，不新建视图。保留Go长度检查，没有unsafe指针写出或忽略内部不一致。输入/描述编码期间仍遵守原不并发修改约定；没有缓存消息标量。

公开输出len/cap、独占所有权、预算及错误顺序保持；通用记录、布局预检与Pack/Decode不改。

## 验证和测量

两版分别通过三模式nil/稀疏tag/inline与4/8整数/描述变化/跨布局/预算失败契约，开启race/checkptr；正常入口分配parity、fixture、所有计时项wire/cap及vet/build均通过。没有新增同底层重复夹具。

同二进制静态调用树，Size/FieldSize/RecordSize与写出树各自静态绑定，只在外层选择，layout不在内部选择器逃逸。大型、records7、prefix31的Encode/PEncode，self/clone/两遍候选；固定一种摆放。七批基线中位+三批pilot校准（60ms目标/40ms下限，最多三阶段，256～65536调用），所有校准和正式GC/分配/长尾保留；六轮×四组ABBA/BAAB，等调用数，双侧50ms预热，GOMAXPROCS1/GOGC100/GOMEMLIMIToff，不扣控制。

第二版完成全部准备后单独运行time.py，首尾核对源码/fixture/生成文件/二进制哈希，禁止覆盖已有计时样本。前后两版在计时期间均未重新编译。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-record-header-output-cursor/prepare.py --prepare-only --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-header-output-cursor/time.py tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-header-output-cursor/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-header-output-cursor/check-calibration.py
```

复现目录须不存在，重测前另存报告原始样本。第一版可用initial-prepare.py.txt传给Python复现准备流程，不运行time.py。

## 结果与决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Encode | −1.26% / −2.53% | −0.21% / +0.02% |
| 大型PEncode | +2.26% / +3.54% | +0.69% / −0.03% |
| 7项记录Encode | +1.00% / +3.27% | +0.84% / +0.18% |
| 7项记录PEncode | +0.64% / +0.55% | −0.43% / −1.23% |
| prefix31 Encode | +0.50% / +3.74% | +1.11% / +0.88% |
| prefix31 PEncode | −0.25% / +0.01% | +1.40% / +1.29% |

正数为耗时增加。暂不采用：大型Encode两遍中位改善是局部证据，第二遍累计接近持平略慢；大型PEncode中位均慢，短记录Encode亦两遍略慢。不是按最小收益比例拒绝，也未丢弃大型Encode的正面数据。固定两字节定位与剩余游标的更新/检查存在取舍，不能以检查点少一个推断整条路径更快。

大型Encode clone中位−0.82%、IQR2.48pp，PEncode clone−1.81%、IQR1.49pp；7项Encode clone+3.07%、IQR2.56pp。所有控制与累计反向保留，不扣减控制、不指定硬件/GC根因。

第二版24进程、144轮、576四元组、2304块，40条记录流程成功；首版另有16条准备流程、没有计时，合计56条准备/测量流程，均成功。240个校准批次独立复算通过。部分7项记录Encode校准达到65536调用上限后pilot未达40ms，限制保留；全部正式块最短31.922ms、中位59.191ms、最长154.588ms，低于20ms为0、低于40ms为374。

正常入口分配parity保持，大型Encode/PEncode3968B/2alloc，wire和返回len/cap保持。实际库9048cfa、GSR正式依赖和七项无关删除不动。因不落地生产，不重复全库、官方oracle、fuzz或Go/TS生产验收；局部三模式契约、race/checkptr、正常分配、fixture/vet与hash核对已完成。
