import request from './request';

export function alistGet() {
  return request.get('/alist');
}

export function alistGetPath(alistId: number | string, path: string) {
  return request.get('/alist', { params: { alistId, path } });
}

export function alistPost(data: Record<string, unknown>) {
  return request.post('/alist', data);
}

export function alistPut(data: Record<string, unknown>) {
  return request.put('/alist', data);
}

export function alistDelete(id: number | string) {
  return request.delete('/alist', { params: { id } });
}
