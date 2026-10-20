# 整数记录字段引用完整37项对照

负值更快。这里是完整Encode/PEncode，未拆成纯写出；计入预检与分配。中位数和累计差反向不删除，控制不作噪声扣除。

| 入口 | 第1轮中位差 | 第2轮中位差 | 第1轮累计差ns | 第2轮累计差ns | self/clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| addressbook/sproto_encode | +3.50% | +2.68% | +21.0 | +141.8 | False/True |
| addressbook/sproto_pack | +0.16% | +3.17% | +0.9 | +56.6 | True/True |
| addressbook/sproto_pencode | +3.68% | +2.17% | +39.5 | +17.6 | False/True |
| game_small/sproto_encode | +5.45% | +5.83% | +34.5 | -26.3 | False/False |
| game_small/sproto_pack | -0.39% | -3.04% | -0.9 | -25.4 | True/True |
| game_small/sproto_pencode | +4.37% | +5.20% | -26.3 | -14.4 | False/False |
| game_large/sproto_encode | +9.03% | +15.75% | +400.7 | +2883.7 | True/False |
| game_large/sproto_pack | +0.12% | -0.72% | +10.3 | -3.0 | True/True |
| game_large/sproto_pencode | +6.47% | +7.34% | +1763.0 | -219.8 | True/False |
| int32_edges/sproto_encode | +0.70% | -4.01% | +9.5 | -821.5 | True/True |
| int32_edges/sproto_pack | +0.11% | -1.11% | +77.6 | +28.9 | True/True |
| int32_edges/sproto_pencode | -0.48% | -1.76% | -15.7 | -360.3 | False/True |
| echo_ascii4k/sproto_encode | +0.74% | -0.86% | -49.2 | +38.9 | True/False |
| echo_ascii4k/sproto_pack | +0.68% | -3.19% | +0.5 | -96.1 | True/True |
| echo_ascii4k/sproto_pencode | -1.29% | -0.14% | +3.3 | -33.9 | True/False |
| ascii32/frame | +0.77% | -0.52% | +10.7 | -25.1 | False/True |
| utf8_1k/frame | -0.34% | +0.74% | -13.4 | +5.8 | True/False |
| ascii4k/frame | -1.50% | +1.36% | -309.0 | +26.1 | True/True |
| zeros4k/frame | +1.38% | -0.84% | -38.0 | -96.2 | False/False |
| flat/full/32/encode | +8.17% | +7.36% | +102.3 | +27.7 | True/False |
| flat/full/32/pencode | -15.47% | +4.57% | -1092.8 | +83.5 | True/False |
| flat/full/33/encode | +11.15% | +10.98% | +263.1 | +158.0 | True/False |
| flat/full/33/pencode | +4.71% | +3.95% | -0.7 | +75.7 | True/False |
| flat/full/4096/encode | +7.92% | +8.25% | +11948.4 | -1813.8 | True/False |
| flat/full/4096/pencode | +7.88% | +5.65% | +13079.3 | +21438.2 | False/False |
| flat/nil/4096/encode | +10.37% | +11.13% | +4250.3 | +2066.4 | True/True |
| flat/nil/4096/pencode | +8.77% | +9.36% | +4341.8 | +6107.3 | True/True |
| flat/half/4096/encode | +9.27% | +8.61% | +7173.2 | +5512.5 | False/False |
| flat/half/4096/pencode | +6.24% | +7.41% | +7098.0 | +15668.6 | True/False |
| nested/32x128/encode | +8.71% | +9.01% | +14537.9 | +5315.5 | True/False |
| nested/32x128/pencode | +7.85% | +6.96% | +22158.1 | +20572.6 | True/False |
| array_record/32/encode | +1.73% | +0.78% | +43.2 | -20.5 | True/True |
| array_record/32/pencode | +1.57% | +1.17% | +15.1 | +38.3 | True/True |
| array_record/33/encode | +0.69% | +0.41% | +1.2 | +216.6 | True/True |
| array_record/33/pencode | +0.92% | +1.95% | +90.5 | +50.7 | True/True |
| array_record/4096/encode | +0.83% | +0.70% | +1701.6 | +1732.4 | False/True |
| array_record/4096/pencode | +1.70% | +1.51% | +5693.9 | +31637.2 | True/False |
