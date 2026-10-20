# 原生字段分布完整32项对照

负值更快。这里是完整Encode/PEncode，未拆成纯写出；计入预检与分配。中位数和累计差反向不删除，控制不作噪声扣除。

| 入口 | 第1轮中位差 | 第2轮中位差 | 第1轮累计差ns | 第2轮累计差ns | self/clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| native/8/zero/encode | -2.78% | -1.76% | -7.3 | -2.7 | False/False |
| native/8/zero/pencode | -0.84% | -0.35% | -1.0 | -8.2 | True/True |
| native/8/inline_max/encode | +0.38% | -3.04% | +3.8 | -6.9 | True/False |
| native/8/inline_max/pencode | -1.95% | -1.12% | -7.6 | -17.9 | False/True |
| native/8/external32/encode | -0.63% | -0.74% | -5.6 | +4.0 | False/True |
| native/8/external32/pencode | +1.64% | -1.17% | -18.6 | -1.0 | True/True |
| native/8/negative32/encode | -1.84% | -1.02% | -3.2 | -1.3 | False/True |
| native/8/negative32/pencode | -0.46% | +1.23% | -7.2 | +10.4 | True/False |
| native/8/external64/encode | -0.15% | +0.41% | -2.1 | +0.5 | True/True |
| native/8/external64/pencode | +0.01% | +0.51% | +10.8 | -5.4 | True/True |
| native/8/mixed/encode | -0.57% | -0.77% | -0.9 | -0.6 | True/True |
| native/8/mixed/pencode | -0.35% | +0.80% | -0.9 | +16.2 | False/True |
| native/8/half_missing/encode | -1.00% | -0.10% | -2.3 | +2.0 | False/True |
| native/8/half_missing/pencode | +0.32% | -2.36% | +17.6 | -5.8 | True/True |
| native/8/all_missing/encode | -1.84% | -2.40% | -1.9 | -2.5 | True/False |
| native/8/all_missing/pencode | -1.72% | -0.00% | -4.7 | -1.4 | True/False |
| native/64/zero/encode | -3.03% | -2.44% | -19.9 | -44.5 | False/False |
| native/64/zero/pencode | -2.21% | -2.01% | -21.0 | -88.2 | True/False |
| native/64/inline_max/encode | -1.76% | -2.76% | +3.4 | -29.7 | True/True |
| native/64/inline_max/pencode | -2.87% | -3.61% | -19.6 | +40.5 | False/True |
| native/64/external32/encode | -0.76% | -0.67% | -498.9 | -7.3 | False/True |
| native/64/external32/pencode | +1.25% | -1.44% | -20.1 | -100.9 | True/False |
| native/64/negative32/encode | -0.05% | +0.79% | +36.2 | +22.4 | False/True |
| native/64/negative32/pencode | +1.40% | +0.50% | +14.9 | +18.6 | True/True |
| native/64/external64/encode | +0.33% | -0.12% | +32.9 | -1.6 | True/True |
| native/64/external64/pencode | +0.30% | -1.14% | +6.5 | -50.7 | True/True |
| native/64/mixed/encode | -0.77% | +0.38% | -126.4 | +3.3 | True/True |
| native/64/mixed/pencode | -0.14% | -0.84% | +1.7 | -15.3 | True/True |
| native/64/half_missing/encode | -1.53% | -1.21% | -64.7 | -6.8 | True/False |
| native/64/half_missing/pencode | -2.15% | -2.58% | -12.9 | -31.4 | True/True |
| native/64/all_missing/encode | -0.01% | -0.99% | +3.1 | -4.7 | True/True |
| native/64/all_missing/pencode | +0.35% | +0.37% | +12.8 | +142.3 | True/True |
