import request from './request';
import type { AlistItem, ApiResponse, PathItem } from '../types';

export function alistGet() {
  return request.get('/alist') as Promise<ApiResponse<AlistItem[]>>;
}

export function alistGetPath(alistId: number | string, path: string) {
  return request.get('/alist', { params: { alistId, path } }) as Promise<ApiResponse<PathItem[]>>;
}

export function alistPost(data: Record<string, unknown>) {
  return request.post('/alist', data) as Promise<ApiResponse<null>>;
}

export function alistPut(data: Record<string, unknown>) {
  return request.put('/alist', data) as Promise<ApiResponse<null>>;
}

export function alistDelete(id: number | string) {
  return request.delete('/alist', { params: { id } }) as Promise<ApiResponse<null>>;
}
