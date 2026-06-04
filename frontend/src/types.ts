export interface ApiResponse<T> {
  code: number;
  data: T;
  msg: string;
}

export interface PageData<T> {
  dataList: T[];
  count: number;
}

export interface UserInfo {
  id: number;
  userName: string;
  createTime: number;
}

export interface AlistItem {
  id: number;
  remark?: string | null;
  url: string;
  userName: string;
  createTime?: number;
}

export interface PathItem {
  path?: string;
  name?: string;
}

export interface JobItem {
  id: number;
  enable: number;
  remark?: string | null;
  srcPath: string;
  dstPath: string;
  alistId: number;
  useCacheT: number | boolean;
  scanIntervalT?: number;
  useCacheS: number | boolean;
  scanIntervalS?: number;
  method: number;
  interval: number;
  isCron: number;
  year?: string | null;
  month?: string | null;
  day?: string | null;
  week?: string | null;
  day_of_week?: string | null;
  hour?: string | null;
  minute?: string | null;
  second?: string | null;
  start_date?: string | null;
  end_date?: string | null;
  exclude?: string | null;
  createTime?: number;
}

export interface JobFormValues {
  alistId: number;
  srcPath: string | string[];
  dstPath: string | string[];
  remark?: string | null;
  method: number;
  isCron: number;
  interval?: number;
  enable: boolean;
  useCacheS?: boolean;
  useCacheT?: boolean;
  scanIntervalS?: number;
  scanIntervalT?: number;
  second?: string;
  minute?: string;
  hour?: string;
  day?: string;
  month?: string;
  day_of_week?: string;
  exclude?: string | null;
}

export interface TreeNode {
  title: string;
  value: string;
  key: string;
  isLeaf?: boolean;
  children?: TreeNode[];
}

export interface TaskRecord {
  id: number;
  status: number;
  runTime?: number;
  successNum?: number;
  failNum?: number;
  allNum?: number;
  createTime?: number;
}

export interface TaskItem {
  id?: number | string;
  taskId?: number;
  srcPath?: string | null;
  dstPath?: string | null;
  isPath?: number;
  fileName?: string | null;
  fileSize?: number | null;
  type?: number;
  alistTaskId?: string | null;
  status: number;
  progress?: number | string;
  errMsg?: string | null;
  createTime?: number;
}

export type TaskNumKey = 'wait' | 'running' | 'success' | 'fail' | 'other';

export interface ScanProgress {
  scannedDirs: number;
  remainingDirs: number;
  totalDirs: number;
}

export interface CurrentTaskData {
  scanFinish: boolean;
  scan?: ScanProgress;
  doingTask: TaskItem[];
  createTime: number;
  duration: number;
  firstSync?: number | null;
  num: Record<TaskNumKey, number>;
  size: Record<TaskNumKey, number>;
}

export interface CurrentTaskView extends CurrentTaskData {
  remainSize: number;
  doneSize: number;
  speed: number;
  speedAvg: number;
  remainTime: number;
}

export interface NotifyItem {
  id: number;
  enable: number;
  method: number;
  params: string;
  createTime?: number;
}

export interface NotifyFormValues {
  method: number;
  enable: boolean;
  url?: string;
  webhook?: string;
  httpMethod?: string;
  methodName?: string;
  contentType?: string;
  needContent?: boolean;
  titleName?: string;
  contentName?: string;
  notSendNull?: boolean;
  sendKey?: string;
  version?: string;
  corpid?: string;
  corpId?: string;
  corpsecret?: string;
  corpSecret?: string;
  agentid?: string;
  agentId?: string;
  touser?: string;
  toUser?: string;
}
