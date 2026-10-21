# 整数记录直接消费：加长块补充测量

基线gosproto f5de803，候选与原实验完全一致。60ms目标块只复核game_small/game_large Decode/PDecode，4模式16进程，两轮保留所有GC/长尾。完整判断、验收与限制见[主报告](../2026-10-21-gosproto-record-scalar/RESULTS.md)，逐项值见[ALL-CASES](ALL-CASES.md)。不单独用加长结果替换原反向数据，不减去控制偏差。
