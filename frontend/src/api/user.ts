import request from './request';

export function login(data: { userName: string; passwd: string }) {
  return request.post('/noAuth/login', data);
}

export function logout() {
  return request.delete('/noAuth/login');
}

export function resetPwd(data: { userName: string; key: string; passwd?: string }) {
  return request.put('/noAuth/login', data);
}

export function getUser() {
  return request.get('/user');
}

export function editPwd(data: { passwd: string; oldPasswd: string }) {
  return request.put('/user', data);
}

export function getLanguage() {
  return request.get('/language');
}

export function setLanguage(data: { language: string }) {
  return request.post('/language', data);
}
