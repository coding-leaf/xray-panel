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
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error.response?.data?.error || error.message)
  }
)

const api = {
  get: (url: string, config?: any) => {
    if (isMockMode()) {
      return handleMockRequest(url, 'GET')
    }
    return instance.get(url, config)
  },
  post: (url: string, data?: any, config?: any) => {
    if (isMockMode()) {
      return handleMockRequest(url, 'POST', data)
    }
    return instance.post(url, data, config)
  },
  put: (url: string, data?: any, config?: any) => {
    if (isMockMode()) {
      return handleMockRequest(url, 'PUT', data)
    }
    return instance.put(url, data, config)
  },
  delete: (url: string, config?: any) => {
    if (isMockMode()) {
      return handleMockRequest(url, 'DELETE')
    }
    return instance.delete(url, config)
  },
}

export default api
