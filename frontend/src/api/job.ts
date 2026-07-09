import request from './request';
import type { ApiResponse, CurrentTaskData, JobItem, PageData, TaskItem, TaskRecord } from '../types';

type RequestOptions = {
  signal?: AbortSignal;
  silent?: boolean;
};

const withRequestOptions = (params: Record<string, unknown>, options?: RequestOptions) => ({
  params,
  ...(options?.signal ? { signal: options.signal } : {}),
  ...(options?.silent ? { silent: true } : {}),
});

export function jobGetJob(params: Record<string, unknown>, options?: RequestOptions) {
  return request.get('/job', withRequestOptions(params, options)) as Promise<ApiResponse<PageData<JobItem>>>;
}

export function jobPost(data: Record<string, unknown>, options?: RequestOptions) {
  return request.post('/job', data, {
    ...(options?.signal ? { signal: options.signal } : {}),
    ...(options?.silent ? { silent: true } : {}),
  }) as Promise<ApiResponse<null>>;
}

export function jobPut(data: Record<string, unknown>) {
  return request.put('/job', data) as Promise<ApiResponse<null>>;
}

export function jobDelete(data: Record<string, unknown>) {
  return request.delete('/job', { params: data }) as Promise<ApiResponse<null>>;
}

export function jobGetTaskCurrent(params: Record<string, unknown>, options?: RequestOptions) {
  return request.get('/job', withRequestOptions({ ...params, current: 1 }, options)) as Promise<ApiResponse<CurrentTaskData | PageData<TaskItem> | TaskItem[] | null>>;
}

export function jobGetTask(params: Record<string, unknown>, options?: RequestOptions) {
  return request.get('/job', withRequestOptions(params, options)) as Promise<ApiResponse<PageData<TaskRecord>>>;
}

export function jobDeleteTask(taskId: number | string) {
  return request.delete('/job', { params: { taskId } }) as Promise<ApiResponse<null>>;
}

export function jobTaskAction(taskId: number | string, action: 'stop' | 'retry') {
  return request.put('/job', { taskId: String(taskId), action }) as Promise<ApiResponse<null>>;
}

export function jobGetTaskItem(params: Record<string, unknown>, options?: RequestOptions) {
  return request.get('/job', withRequestOptions(params, options)) as Promise<ApiResponse<PageData<TaskItem>>>;
}
