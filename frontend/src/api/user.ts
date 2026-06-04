import request from './request';
import type { ApiResponse, UserInfo } from '../types';

export function login(data: { userName: string; passwd: string }) {
  return request.post('/noAuth/login', data) as Promise<ApiResponse<UserInfo>>;
}

export function logout() {
  return request.delete('/noAuth/login') as Promise<ApiResponse<null>>;
}

export function resetPwd(data: { userName: string; key: string; passwd?: string }) {
  return request.put('/noAuth/login', data) as Promise<ApiResponse<string | null>>;
}

export function getUser() {
  return request.get('/user') as Promise<ApiResponse<UserInfo>>;
}

export function editPwd(data: { passwd: string; oldPasswd: string }) {
  return request.put('/user', data) as Promise<ApiResponse<null>>;
}

export function getLanguage() {
  return request.get('/language') as Promise<ApiResponse<string>>;
}

export function setLanguage(data: { language: string }) {
  return request.post('/language', data) as Promise<ApiResponse<null>>;
}
