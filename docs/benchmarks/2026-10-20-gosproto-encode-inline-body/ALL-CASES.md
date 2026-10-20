# 固定19项完整对照

负值表示候选更快。中位比为六轮 ABBA/BAAB 的描述统计，累计差以全部块总耗时/调用数计算；二者反向不删除。Pack 是未修改算法的对照，不将其浮动算作 Encode 收益。

| 项目 | 第1轮中位差 | 第2轮中位差 | 第1轮累计差 ns | 第2轮累计差 ns | self/clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| addressbook/sproto_encode | +0.37% | -0.39% | +18.8 | -7.1 | False/True |
| addressbook/sproto_pack | +0.60% | -0.74% | +0.8 | -2.5 | True/True |
| addressbook/sproto_pencode | -0.06% | -2.11% | -0.8 | -17.2 | True/True |
| game_small/sproto_encode | +0.50% | -0.40% | +31.5 | +0.3 | True/True |
| game_small/sproto_pack | -0.38% | -0.52% | -0.9 | -0.9 | True/True |
| game_small/sproto_pencode | -2.03% | +0.64% | -24.2 | +18.5 | False/True |
| game_large/sproto_encode | -0.36% | -1.36% | +214.9 | -57.1 | True/True |
| game_large/sproto_pack | -1.16% | +0.22% | -87.4 | -45.1 | True/False |
| game_large/sproto_pencode | +2.23% | -0.67% | +138.6 | -93.3 | True/True |
| int32_edges/sproto_encode | -0.04% | -0.37% | +148.5 | -1.3 | True/False |
| int32_edges/sproto_pack | -0.17% | +2.05% | -3.4 | +37.4 | True/True |
| int32_edges/sproto_pencode | +0.84% | +0.00% | +371.3 | +71.7 | True/True |
| echo_ascii4k/sproto_encode | +1.17% | +0.37% | -179.1 | +40.0 | False/True |
| echo_ascii4k/sproto_pack | -0.24% | -0.01% | -5.4 | +2.8 | False/True |
| echo_ascii4k/sproto_pencode | -0.58% | +0.24% | -5.3 | +17.8 | True/False |
| ascii32/frame | -0.74% | -2.25% | -12.4 | -7.4 | False/True |
| utf8_1k/frame | -2.33% | -1.32% | -24.9 | -36.4 | True/True |
| ascii4k/frame | +0.28% | -1.89% | -86.6 | -88.5 | True/True |
| zeros4k/frame | +0.46% | -0.36% | +15.2 | +15.7 | False/False |
