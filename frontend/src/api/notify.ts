import request from './request';

export function notifyGet() {
  return request.get('/notify');
}

export function notifyPost(data: Record<string, unknown>) {
  return request.post('/notify', data);
}

export function notifyPut(data: Record<string, unknown>) {
  return request.put('/notify', data);
}

export function notifyDelete(notifyId: number | string) {
  return request.delete('/notify', { params: { notifyId } });
}
