# 整数记录指针读取完整37项对照

负值更快。包含原19项Encode/Pack/PEncode/manual帧及18项增长Encode/PEncode；完整编码入口计入预检与分配，未拆成纯写出。中位数和累计差反向不删除，控制不作噪声扣除。

| 入口 | 第1轮中位差 | 第2轮中位差 | 第1轮累计差ns | 第2轮累计差ns | self/clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| addressbook/sproto_encode | -0.28% | +1.57% | -82.4 | +21.6 | False/False |
| addressbook/sproto_pack | -1.92% | -0.16% | -8.5 | -0.2 | True/False |
| addressbook/sproto_pencode | +6.43% | +0.43% | +55.5 | -7.7 | False/True |
| game_small/sproto_encode | -7.02% | -4.90% | -51.3 | -43.0 | False/True |
| game_small/sproto_pack | +1.66% | -0.24% | +1.9 | +24.8 | False/True |
| game_small/sproto_pencode | -5.97% | -3.92% | -305.9 | -39.1 | False/True |
| game_large/sproto_encode | -18.63% | -19.05% | -983.0 | -616.2 | False/True |
| game_large/sproto_pack | -1.06% | +0.47% | +904.7 | +16.2 | True/True |
| game_large/sproto_pencode | -15.87% | -14.34% | -1824.2 | -747.4 | False/True |
| int32_edges/sproto_encode | +1.61% | +2.01% | -59.8 | +154.6 | True/True |
| int32_edges/sproto_pack | +2.88% | +0.91% | +87.1 | -194.8 | True/True |
| int32_edges/sproto_pencode | -3.74% | +2.00% | -292.2 | -505.6 | True/False |
| echo_ascii4k/sproto_encode | +0.69% | +0.09% | +4.2 | -26.1 | False/False |
| echo_ascii4k/sproto_pack | +0.86% | -0.01% | +134.0 | -69.1 | True/False |
| echo_ascii4k/sproto_pencode | -2.25% | +0.24% | +111.3 | -1.3 | True/False |
| ascii32/frame | +0.52% | +1.15% | +3.7 | +18.4 | True/False |
| utf8_1k/frame | -0.48% | +0.68% | -46.8 | +5.6 | True/False |
| ascii4k/frame | +0.85% | +0.69% | +15.1 | +566.4 | True/False |
| zeros4k/frame | +1.08% | +0.61% | +100.5 | +176.5 | True/False |
| flat/full/32/encode | -13.32% | -11.46% | -192.1 | -19.3 | False/True |
| flat/full/32/pencode | -10.40% | -8.95% | -134.8 | -157.1 | True/True |
| flat/full/33/encode | -13.21% | -13.69% | -229.2 | -206.3 | False/True |
| flat/full/33/pencode | -7.83% | -9.67% | -243.6 | -97.0 | True/False |
| flat/full/4096/encode | -16.33% | -15.35% | -26263.0 | -41035.5 | True/True |
| flat/full/4096/pencode | -12.75% | -12.14% | -27194.5 | -26738.3 | False/True |
| flat/nil/4096/encode | -37.60% | -37.86% | -21385.9 | -24703.6 | False/True |
| flat/nil/4096/pencode | -34.08% | -33.91% | -28617.4 | -24909.2 | True/True |
| flat/half/4096/encode | -22.89% | -22.96% | -26410.9 | -28660.7 | True/True |
| flat/half/4096/pencode | -15.69% | -18.58% | -16829.3 | -26730.2 | True/True |
| nested/32x128/encode | -14.38% | -14.50% | -30856.3 | -24354.6 | False/True |
| nested/32x128/pencode | -9.80% | -10.88% | -22896.1 | -25211.5 | False/True |
| array_record/32/encode | -0.21% | +0.23% | +607.9 | +150.0 | True/True |
| array_record/32/pencode | +0.48% | +0.11% | -214.8 | +3.8 | True/True |
| array_record/33/encode | +0.49% | +0.21% | +11.2 | +23.8 | False/True |
| array_record/33/pencode | -2.55% | +0.20% | -299.8 | +2.1 | True/False |
| array_record/4096/encode | +1.22% | +0.94% | +3669.7 | -15594.5 | True/True |
| array_record/4096/pencode | +0.19% | -0.32% | -12047.1 | -15337.6 | True/False |
