# 完整配对结果

六轮配对B/A中位数；负值更快。累计Δns由全部保留块的总时间/调用数计算，不扣控制。

| 入口 | 第1轮变化 | 第2轮变化 | 第1轮累计Δns | 第2轮累计Δns | self / clone |
| --- | ---: | ---: | ---: | ---: | --- |
| shared/game_large/align0_spare0/unpack | -13.36% | -11.51% | -882.0 | -561.4 | 失败 / 失败 |
| shared/game_large/align0_spare0/pdecode | -4.42% | -1.83% | -925.5 | +333.3 | 通过 / 通过 |
| shared/game_large/align0_spare64/unpack | -12.66% | -13.14% | -990.1 | -619.5 | 通过 / 失败 |
| shared/game_large/align0_spare64/pdecode | -5.09% | -4.42% | -1966.2 | -1298.4 | 通过 / 失败 |
| shared/game_large/align1_spare0/unpack | -13.29% | -12.61% | -644.4 | -625.6 | 失败 / 失败 |
| shared/game_large/align1_spare0/pdecode | -3.05% | -3.41% | -8496.1 | -809.2 | 失败 / 失败 |
| shared/game_large/align1_spare64/unpack | -12.05% | -11.92% | -574.6 | -599.0 | 失败 / 失败 |
| shared/game_large/align1_spare64/pdecode | -2.70% | -6.91% | -753.6 | -1134.7 | 失败 / 失败 |
| shared/int32_edges/align0_spare0/unpack | -13.90% | -13.42% | -806.2 | -672.9 | 通过 / 失败 |
| shared/int32_edges/align0_spare0/pdecode | -9.85% | -10.40% | -698.6 | -568.9 | 失败 / 失败 |
| shared/int32_edges/align0_spare64/unpack | -13.59% | -12.67% | -879.9 | -655.2 | 通过 / 失败 |
| shared/int32_edges/align0_spare64/pdecode | -10.43% | -10.79% | -779.3 | -1164.0 | 通过 / 失败 |
| shared/int32_edges/align1_spare0/unpack | -11.60% | -13.00% | -544.1 | -610.4 | 通过 / 通过 |
| shared/int32_edges/align1_spare0/pdecode | -8.17% | -11.18% | -659.3 | -792.4 | 通过 / 通过 |
| shared/int32_edges/align1_spare64/unpack | -11.76% | -11.63% | -503.5 | -643.1 | 失败 / 通过 |
| shared/int32_edges/align1_spare64/pdecode | -10.42% | -9.47% | -725.0 | -723.5 | 通过 / 通过 |
| shared/width8_len896/align0_spare0/unpack | -8.49% | -8.44% | -323.1 | -445.2 | 失败 / 失败 |
| shared/width8_len896/align0_spare0/pdecode | -8.17% | -8.12% | -815.7 | -854.3 | 通过 / 失败 |
| shared/width8_len896/align0_spare64/unpack | -9.10% | -8.34% | -729.8 | -706.0 | 通过 / 失败 |
| shared/width8_len896/align0_spare64/pdecode | -8.97% | -7.81% | -640.5 | -101.2 | 通过 / 通过 |
| shared/width8_len896/align1_spare0/unpack | -9.32% | -8.23% | -770.3 | -505.8 | 失败 / 失败 |
| shared/width8_len896/align1_spare0/pdecode | -9.51% | -7.33% | -921.3 | -429.0 | 通过 / 失败 |
| shared/width8_len896/align1_spare64/unpack | -8.56% | -8.57% | -719.7 | -677.1 | 通过 / 通过 |
| shared/width8_len896/align1_spare64/pdecode | -5.60% | -8.59% | -504.4 | -757.1 | 失败 / 通过 |
| shared/nonzero1x256/align0_spare0/unpack | +31.72% | +32.04% | +396.1 | +395.1 | 失败 / 失败 |
| shared/nonzero1x256/align0_spare64/unpack | +31.01% | +36.65% | +363.5 | +1821.7 | 失败 / 失败 |
| shared/nonzero1x256/align1_spare0/unpack | +33.67% | +32.13% | +434.6 | +382.7 | 通过 / 失败 |
| shared/nonzero1x256/align1_spare64/unpack | +30.66% | +30.19% | +384.8 | +337.9 | 失败 / 失败 |
| shared/nonzero2x256/align0_spare0/unpack | +28.98% | +22.98% | +411.8 | +346.1 | 失败 / 失败 |
| shared/nonzero2x256/align0_spare64/unpack | +25.60% | +32.30% | +370.8 | +469.6 | 通过 / 失败 |
| shared/nonzero2x256/align1_spare0/unpack | +28.07% | +17.37% | +392.6 | +253.1 | 通过 / 通过 |
| shared/nonzero2x256/align1_spare64/unpack | +43.09% | +30.42% | +4500.0 | +427.4 | 通过 / 失败 |
| shared/FF512-sparse4096/align0_spare0/unpack | +8.00% | +8.98% | +213.0 | +304.4 | 通过 / 失败 |
| shared/FF512-sparse4096/align0_spare64/unpack | +10.03% | +8.68% | +333.6 | +196.9 | 失败 / 失败 |
| shared/FF512-sparse4096/align1_spare0/unpack | +7.14% | +11.72% | +359.6 | +375.3 | 失败 / 失败 |
| shared/FF512-sparse4096/align1_spare64/unpack | +9.13% | +12.84% | +315.4 | +538.2 | 通过 / 失败 |
| shared/zeroRun8x64/align0_spare0/unpack | -2.05% | +2.27% | -32.5 | +103.8 | 通过 / 失败 |
| shared/zeroRun8x64/align0_spare64/unpack | +0.22% | -0.79% | +8.5 | -2.8 | 失败 / 失败 |
| shared/zeroRun8x64/align1_spare0/unpack | +0.58% | -2.55% | +7.7 | -29.2 | 失败 / 失败 |
| shared/zeroRun8x64/align1_spare64/unpack | -3.50% | +0.87% | -156.6 | -77.3 | 通过 / 失败 |
