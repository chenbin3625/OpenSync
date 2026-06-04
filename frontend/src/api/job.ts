import request from './request';
import type { ApiResponse, CurrentTaskData, JobItem, PageData, TaskItem, TaskRecord } from '../types';

export function jobGetJob(params: Record<string, unknown>) {
  return request.get('/job', { params }) as Promise<ApiResponse<PageData<JobItem>>>;
}

export function jobPost(data: Record<string, unknown>) {
  return request.post('/job', data) as Promise<ApiResponse<null>>;
}

export function jobPut(data: Record<string, unknown>) {
  return request.put('/job', data) as Promise<ApiResponse<null>>;
}

export function jobDelete(data: Record<string, unknown>) {
  return request.delete('/job', { params: data }) as Promise<ApiResponse<null>>;
}

export function jobGetTaskCurrent(params: Record<string, unknown>) {
  return request.get('/job', { params: { ...params, current: 1 } }) as Promise<ApiResponse<CurrentTaskData | TaskItem[] | null>>;
}

export function jobGetTask(params: Record<string, unknown>) {
  return request.get('/job', { params }) as Promise<ApiResponse<PageData<TaskRecord>>>;
}

export function jobDeleteTask(taskId: number | string) {
  return request.delete('/job', { params: { taskId } }) as Promise<ApiResponse<null>>;
}

export function jobGetTaskItem(params: Record<string, unknown>) {
  return request.get('/job', { params }) as Promise<ApiResponse<PageData<TaskItem>>>;
}
