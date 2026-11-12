import api from './index'

export interface BatchDeleteUsersResponse {
  message: string
}

export const batchDeleteUsers = async (ids: number[]): Promise<BatchDeleteUsersResponse> => {
  return api.post<BatchDeleteUsersResponse>('/users/batch-delete', { ids })
}
