# 52项完整结果

每项六轮；中位变化为配对时间B/A−1，累计差为所有保留块的总耗时/调用数之差。不同统计量可能反向；控制标记不用于扣除。单位ns。

| 入口 | 第1轮变化 | 第2轮变化 | 第1轮累计Δns | 第2轮累计Δns | self / clone |
| --- | ---: | ---: | ---: | ---: | --- |
| body/addressbook/decode | -0.00% | +1.56% | -467.8 | +100.2 | 失败 / 通过 |
| body/addressbook/unpack | +1.65% | +0.30% | -3.6 | -3.2 | 失败 / 失败 |
| body/addressbook/pdecode | +0.44% | -0.00% | +4.5 | +3.1 | 失败 / 通过 |
| rpc/addressbook/dispatch | +0.93% | -0.92% | -65.1 | -4.8 | 失败 / 通过 |
| body/game_small/decode | +0.15% | -2.30% | +13.9 | -84.0 | 失败 / 通过 |
| body/game_small/unpack | -0.29% | +0.12% | -0.4 | +5.1 | 失败 / 失败 |
| body/game_small/pdecode | -0.04% | -1.81% | +46.2 | -270.4 | 通过 / 通过 |
| rpc/game_small/dispatch | -1.19% | -1.68% | -41.8 | -31.4 | 失败 / 通过 |
| body/game_large/decode | -1.77% | +2.47% | -481.4 | -54806.3 | 通过 / 失败 |
| body/game_large/unpack | +1.03% | -0.08% | +57.4 | -15.3 | 失败 / 通过 |
| body/game_large/pdecode | -1.37% | +0.12% | -586.9 | +287.5 | 失败 / 失败 |
| rpc/game_large/dispatch | -0.94% | +0.41% | -1838.9 | +153.6 | 失败 / 失败 |
| body/int32_edges/decode | -25.88% | -26.84% | -141.4 | -781.1 | 失败 / 失败 |
| body/int32_edges/unpack | -1.46% | -0.29% | +21.5 | -54.7 | 通过 / 通过 |
| body/int32_edges/pdecode | -6.86% | -8.44% | -575.7 | -756.6 | 失败 / 失败 |
| rpc/int32_edges/dispatch | -8.77% | -8.76% | -767.9 | -661.5 | 失败 / 失败 |
| body/echo_ascii4k/decode | +0.85% | +2.66% | +2.6 | +14.0 | 失败 / 失败 |
| body/echo_ascii4k/unpack | -0.74% | -2.37% | -26.4 | -65.8 | 失败 / 通过 |
| body/echo_ascii4k/pdecode | +1.16% | -3.84% | +12.2 | -33.7 | 通过 / 通过 |
| rpc/echo_ascii4k/dispatch | +0.73% | -3.16% | +116.7 | -196.5 | 失败 / 失败 |
| echo/ascii32/payload_decode | +3.21% | -0.06% | +5.2 | -0.4 | 通过 / 通过 |
| echo/ascii32/frame_decode | -0.05% | -0.33% | -11.6 | +30.2 | 通过 / 失败 |
| echo/utf8_1k/payload_decode | +1.87% | +0.88% | -33.5 | +4.8 | 通过 / 通过 |
| echo/utf8_1k/frame_decode | +0.99% | +1.83% | -6.9 | -28.3 | 通过 / 通过 |
| echo/ascii4k/payload_decode | -0.87% | -0.40% | -8.3 | -8.8 | 失败 / 失败 |
| echo/ascii4k/frame_decode | -1.21% | +0.96% | +276.8 | +31.2 | 通过 / 失败 |
| echo/zeros4k/payload_decode | +1.64% | +0.12% | +14.3 | -22.4 | 失败 / 失败 |
| echo/zeros4k/frame_decode | +1.66% | +0.04% | +95.9 | +2.0 | 通过 / 失败 |
| growth/flat/full/4096/decode | -0.35% | +0.36% | -1988.3 | +188441.1 | 失败 / 失败 |
| growth/flat/full/4096/pdecode | +0.72% | -0.32% | +645.1 | -3154.9 | 失败 / 失败 |
| growth/flat/nil/4096/decode | -1.09% | +0.81% | +870.3 | +31788.8 | 通过 / 通过 |
| growth/flat/nil/4096/pdecode | -0.64% | -3.71% | -9816.0 | -24025.7 | 通过 / 通过 |
| growth/flat/half/4096/decode | +0.06% | +1.58% | -1452.3 | +5075.4 | 失败 / 失败 |
| growth/flat/half/4096/pdecode | +1.15% | +1.56% | +6956.7 | +5450.1 | 失败 / 失败 |
| growth/nested/32x128/decode | -0.71% | +0.18% | -3432.8 | -106.1 | 失败 / 失败 |
| growth/nested/32x128/pdecode | +1.43% | -0.06% | +38422.3 | +39583.3 | 失败 / 失败 |
| growth/array_record/4096/decode | -0.75% | -3.69% | +20987.1 | -25589.2 | 通过 / 失败 |
| growth/array_record/4096/pdecode | -1.17% | -0.09% | +6232.6 | -1131.3 | 通过 / 通过 |
| array/width4_len3/decode | -1.46% | +0.11% | -3.8 | -0.1 | 通过 / 失败 |
| array/width4_len3/unpack | +0.59% | -0.19% | +0.4 | -1.1 | 通过 / 通过 |
| array/width4_len3/pdecode | -0.36% | -2.92% | -2.5 | -5.7 | 通过 / 失败 |
| rpc/width4_len3/dispatch | -2.02% | +2.46% | -33.2 | +101.1 | 通过 / 通过 |
| array/width8_len3/decode | -1.31% | +0.90% | -2.8 | +1.4 | 失败 / 失败 |
| array/width8_len3/unpack | -1.85% | -1.29% | -4.7 | -0.3 | 通过 / 通过 |
| array/width8_len3/pdecode | +1.95% | +0.81% | +2.8 | +23.3 | 失败 / 失败 |
| rpc/width8_len3/dispatch | +0.37% | -1.86% | +10.4 | -9.6 | 通过 / 失败 |
| array/width8_len896/decode | -28.09% | -28.50% | -733.0 | -838.9 | 失败 / 失败 |
| array/width8_len896/unpack | +1.20% | -0.85% | +121.1 | -106.9 | 失败 / 通过 |
| array/width8_len896/pdecode | -1.17% | -7.52% | +7157.1 | -1430.5 | 失败 / 通过 |
| rpc/width8_len896/dispatch | -4.25% | -7.03% | -39.3 | -730.3 | 失败 / 通过 |
| control/game_large/encode | -0.42% | -0.10% | -25.1 | -90.9 | 通过 / 失败 |
| control/game_large/pencode | -0.07% | +1.01% | -37.4 | +224.7 | 失败 / 通过 |
