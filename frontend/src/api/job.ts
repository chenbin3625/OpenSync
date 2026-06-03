import request from './request';

export function jobGetJob(params: Record<string, unknown>) {
  return request.get('/job', { params });
}

export function jobPost(data: Record<string, unknown>) {
  return request.post('/job', data);
}

export function jobPut(data: Record<string, unknown>) {
  return request.put('/job', data);
}

export function jobDelete(data: Record<string, unknown>) {
  return request.delete('/job', { params: data });
}

export function jobGetTaskCurrent(params: Record<string, unknown>) {
  return request.get('/job', { params: { ...params, current: 1 } });
}

export function jobGetTask(params: Record<string, unknown>) {
  return request.get('/job', { params });
}

export function jobDeleteTask(taskId: number | string) {
  return request.delete('/job', { params: { taskId } });
}

export function jobGetTaskItem(params: Record<string, unknown>) {
  return request.get('/job', { params });
}
