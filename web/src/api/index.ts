import axios from 'axios'
import { isMockMode, handleMockRequest } from '../mock'

const instance = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

instance.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

instance.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response && error.response.status === 401) {
      localStorage.removeItem('token')
      if (window.location.pathname !== '/login' && window.location.pathname !== '/portal') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error.response?.data?.error || error.message)
  }
)

const api = {
  get: <T = any>(url: string, config?: any): Promise<T> => {
    if (isMockMode()) {
      return handleMockRequest(url, 'GET') as Promise<T>
    }
    return instance.get(url, config) as Promise<T>
  },
  post: <T = any>(url: string, data?: any, config?: any): Promise<T> => {
    if (isMockMode()) {
      return handleMockRequest(url, 'POST', data) as Promise<T>
    }
    return instance.post(url, data, config) as Promise<T>
  },
  put: <T = any>(url: string, data?: any, config?: any): Promise<T> => {
    if (isMockMode()) {
      return handleMockRequest(url, 'PUT', data) as Promise<T>
    }
    return instance.put(url, data, config) as Promise<T>
  },
  delete: <T = any>(url: string, config?: any): Promise<T> => {
    if (isMockMode()) {
      return handleMockRequest(url, 'DELETE') as Promise<T>
    }
    return instance.delete(url, config) as Promise<T>
  },
}

export * from './reality'
export default api
