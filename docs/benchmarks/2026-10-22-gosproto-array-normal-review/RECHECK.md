# 短数组延长计时复测

每项400ms Go benchmark目标，三轮ABBA/BAAB；不替换首次数据、不扣减self。

| 项目 | self中位 | 候选中位 | 候选三轮 |
| --- | ---: | ---: | --- |
| BenchmarkArrayNormalReview/w4_n1/decode | +0.24% | +0.10% | +0.38% / -0.51% / +0.10% |
| BenchmarkArrayNormalReview/w4_n5/decode | -18.49% | -7.89% | -5.07% / -13.06% / -7.89% |
| BenchmarkArrayNormalReview/w8_n8/decode | -1.85% | -1.40% | -0.00% / -3.72% / -1.40% |
