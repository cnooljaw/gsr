# 整数记录Tag复用完整37项对照

负值更快。这里是完整Encode/PEncode，未拆成纯写出；计入预检与分配。中位数和累计差反向不删除，控制不作噪声扣除。

| 入口 | 第1轮中位差 | 第2轮中位差 | 第1轮累计差ns | 第2轮累计差ns | self/clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| addressbook/sproto_encode | +0.54% | +0.78% | +4.3 | +14.6 | False/True |
| addressbook/sproto_pack | -1.08% | +0.15% | -2.3 | -0.9 | True/True |
| addressbook/sproto_pencode | +0.30% | +1.10% | +1.3 | +23.7 | True/True |
| game_small/sproto_encode | +2.86% | +3.44% | +31.4 | +21.3 | True/False |
| game_small/sproto_pack | -0.16% | -1.23% | -5.4 | -29.2 | False/True |
| game_small/sproto_pencode | +2.48% | +1.56% | +69.1 | +35.5 | True/False |
| game_large/sproto_encode | +6.22% | +7.18% | +252.7 | +965.8 | True/False |
| game_large/sproto_pack | +2.58% | +0.22% | +59.0 | +44.0 | False/True |
| game_large/sproto_pencode | +4.58% | +6.33% | +252.4 | +661.0 | True/False |
| int32_edges/sproto_encode | +1.53% | +1.11% | +48.9 | +159.0 | False/True |
| int32_edges/sproto_pack | -0.38% | -1.18% | -54.7 | -66.8 | True/True |
| int32_edges/sproto_pencode | +0.00% | +0.78% | -195.5 | +18.3 | True/True |
| echo_ascii4k/sproto_encode | -0.42% | +0.42% | -11.2 | +3.1 | False/True |
| echo_ascii4k/sproto_pack | +1.63% | +1.63% | -5.1 | -32.6 | True/True |
| echo_ascii4k/sproto_pencode | -0.25% | -1.25% | -14.9 | -99.0 | False/False |
| ascii32/frame | -1.12% | +0.04% | -32.2 | -9.0 | True/False |
| utf8_1k/frame | -0.58% | -0.58% | -41.8 | -4.6 | True/True |
| ascii4k/frame | -0.51% | -0.96% | -32.4 | -185.9 | True/False |
| zeros4k/frame | +0.07% | +0.49% | +204.3 | +14.0 | False/True |
| flat/full/32/encode | +6.42% | +6.64% | +79.2 | +72.8 | True/False |
| flat/full/32/pencode | +2.12% | +3.85% | -71.0 | +24.9 | True/False |
| flat/full/33/encode | +7.53% | +7.11% | +94.3 | +119.4 | True/False |
| flat/full/33/pencode | +3.62% | +3.37% | +50.3 | +56.7 | True/False |
| flat/full/4096/encode | +4.62% | +6.75% | +7575.0 | +13471.0 | True/False |
| flat/full/4096/pencode | +4.86% | +4.67% | +5072.8 | +4321.6 | True/False |
| flat/nil/4096/encode | +0.22% | +0.21% | +122.8 | +186.6 | True/True |
| flat/nil/4096/pencode | +0.37% | +0.72% | +151.3 | +13896.4 | True/True |
| flat/half/4096/encode | +4.73% | +5.39% | -1409.9 | +4938.5 | False/False |
| flat/half/4096/pencode | +4.44% | +4.94% | +4761.3 | +20913.5 | True/False |
| nested/32x128/encode | +5.96% | +5.11% | +4586.7 | +2985.3 | False/False |
| nested/32x128/pencode | +5.16% | +4.40% | +10783.3 | +8598.8 | True/False |
| array_record/32/encode | +0.42% | +0.86% | +102.0 | +310.9 | True/False |
| array_record/32/pencode | +0.28% | +0.09% | -2550.3 | -45.8 | False/True |
| array_record/33/encode | +0.43% | -0.41% | +8.7 | +73.9 | True/True |
| array_record/33/pencode | +1.20% | +0.30% | -23.0 | +15.2 | True/True |
| array_record/4096/encode | -0.25% | -0.31% | -16379.6 | -968.9 | True/True |
| array_record/4096/pencode | -0.53% | +0.37% | -6591.4 | -6514.1 | True/True |
