# 准备过程记录

手工第一条测试误在GSR目录执行，输出“no tests to run”，不计入有效验证。随后在完整库快照中执行TestUnpackSizeFastDifferential，占位函数明确失败于“128 byte input must use native preflight”；实现后差分及保护页在race/checkptr下通过。

第一版测量驱动在fixture准备阶段失败，尚未计时：上一轮clone归档的validate.py将参考源码快照误命名为experiment.go.txt，覆盖了harness模板。bootstrap-failed.json保存失败命令。已将参考快照改为独立名称，并从上一轮冻结harness恢复模板、核对SHA一致；使用全新输出目录完成测量，未覆盖失败记录或重跑挑选数据。上一轮报告与脚本同时修正，但既有原始数据/二进制未改。
