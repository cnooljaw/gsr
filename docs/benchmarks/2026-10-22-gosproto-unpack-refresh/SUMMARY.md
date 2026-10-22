# 最新Unpack源码采样

单位为秒；所有比例使用完整Total samples。预检flat不含其inline OnesCount8子帧，后者单列；make列为该行累计，其他列为flat，不能直接全部相加得总CPU。

| 消息/入口/遍 | 总采样 | kevent | Unpack flat/cum | 预检flat | OnesCount子帧 | 写出flat | make累计 | 普通字节赋值flat |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| int32_edges/sproto_unpack/pass1 | 2.30 | 0.69 | 1.31/1.59 | 0.62 | 0.16 | 0.69 | 0.12 | 0.47 |
| int32_edges/sproto_pdecode/pass1 | 2.30 | 1.11 | 0.79/0.92 | 0.39 | 0.05 | 0.40 | 0.08 | 0.21 |
| game_large/sproto_unpack/pass1 | 2.35 | 0.62 | 1.50/1.70 | 0.65 | 0.12 | 0.85 | 0.07 | 0.51 |
| game_large/sproto_pdecode/pass1 | 2.31 | 0.59 | 0.42/0.47 | 0.21 | 0.03 | 0.21 | 0.02 | 0.14 |
| addressbook/sproto_unpack/pass1 | 2.20 | 0.54 | 1.03/1.61 | 0.44 | 0.06 | 0.58 | 0.50 | 0.36 |
| addressbook/sproto_pdecode/pass1 | 2.34 | 0.38 | 0.20/0.31 | 0.10 | 0.01 | 0.10 | 0.10 | 0.06 |
| int32_edges/sproto_unpack/pass2 | 2.31 | 0.63 | 1.37/1.63 | 0.58 | 0.09 | 0.79 | 0.16 | 0.48 |
| int32_edges/sproto_pdecode/pass2 | 2.23 | 1.16 | 0.69/0.81 | 0.31 | 0.08 | 0.38 | 0.04 | 0.26 |
| game_large/sproto_unpack/pass2 | 2.32 | 0.56 | 1.55/1.75 | 0.70 | 0.11 | 0.85 | 0.09 | 0.53 |
| game_large/sproto_pdecode/pass2 | 2.32 | 0.68 | 0.45/0.49 | 0.25 | 0.03 | 0.20 | 0.01 | 0.11 |
| addressbook/sproto_unpack/pass2 | 1.98 | 0.58 | 0.88/1.33 | 0.45 | 0.05 | 0.43 | 0.39 | 0.26 |
| addressbook/sproto_pdecode/pass2 | 1.76 | 0.28 | 0.11/0.16 | 0.05 | 0.00 | 0.06 | 0.04 | 0.02 |
