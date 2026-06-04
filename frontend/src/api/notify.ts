import request from './request';
import type { ApiResponse, NotifyItem } from '../types';

export function notifyGet() {
  return request.get('/notify') as Promise<ApiResponse<NotifyItem[]>>;
}

export function notifyPost(data: Record<string, unknown>) {
  return request.post('/notify', data) as Promise<ApiResponse<null>>;
}

export function notifyPut(data: Record<string, unknown>) {
  return request.put('/notify', data) as Promise<ApiResponse<null>>;
}

export function notifyDelete(notifyId: number | string) {
  return request.delete('/notify', { params: { notifyId } }) as Promise<ApiResponse<null>>;
}
