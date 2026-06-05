import request from './request';
import type { ApiResponse, SystemSettings } from '../types';

export function getSystemConfig() {
  return request.get('/system/config') as Promise<ApiResponse<SystemSettings>>;
}

export function updateSystemConfig(data: SystemSettings) {
  return request.put('/system/config', data) as Promise<ApiResponse<SystemSettings>>;
}
