// Package naming 定义 Task 名称与由它派生的 Job / Pod 名称之间的约束。
//
// 这条约束同时被两个地方使用：TaskReconciler（决定能不能建 Job）和
// internal/workflow（Workflow 编排子 Task 时的入口校验），因此单独成包，
// 避免任何一侧复制第二份常量。
package naming

// MaxTaskNameLength 是 Task 名称的长度上限。
//
// 推导：TaskReconciler 用 "<task 名>-<10 位 Unix 秒>" 创建 Job，Job 控制器再给 Pod 追加
// "-<5 位随机后缀>"，而 Pod 名不能超过 63 字符（否则容器运行时拒绝该主机名）。
// 固定开销 1+10+1+5 = 17，因此 Task 名最多 46 字符。
const MaxTaskNameLength = 46
