import request, { type RequestOptions, withRequestOptions } from './request';
import type { ApiResponse, SystemSettings } from '../types';

export function getSystemConfig(options?: RequestOptions) {
  return request.get('/system/config', withRequestOptions({}, options)) as Promise<ApiResponse<SystemSettings>>;
}

export function updateSystemConfig(data: SystemSettings) {
  return request.put('/system/config', data) as Promise<ApiResponse<SystemSettings>>;
}
