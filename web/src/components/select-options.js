export const taskTypes = [
  {
    value: "OneTime",
    label: "一次性执行",
    description: "OneTime · 创建后执行一次",
  },
  { value: "Cron", label: "定时执行", description: "Cron · 按计划重复执行" },
  {
    value: "Delay",
    label: "延迟执行",
    description: "Delay · 等待指定时长后执行",
  },
];
export const taskTypeFilters = [{ value: "", label: "全部类型" }, ...taskTypes];
export const taskPhases = [
  { value: "", label: "全部状态" },
  { value: "Pending", label: "等待中", description: "Pending" },
  { value: "Running", label: "运行中", description: "Running" },
  { value: "Succeeded", label: "已成功", description: "Succeeded" },
  { value: "Failed", label: "已失败", description: "Failed" },
  { value: "Suspended", label: "已暂停", description: "Suspended" },
];
export const workflowPhases = taskPhases.filter(
  (option) => option.value !== "Suspended",
);
export const concurrencyPolicies = [
  {
    value: "Allow",
    label: "允许并发",
    description: "Allow · 多次执行可以同时运行",
  },
  {
    value: "Forbid",
    label: "跳过重叠执行",
    description: "Forbid · 上一轮未结束时跳过",
  },
  {
    value: "Replace",
    label: "替换上一次执行",
    description: "Replace · 停止旧执行并启动新执行",
  },
];
