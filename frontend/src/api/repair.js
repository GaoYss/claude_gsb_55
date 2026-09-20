import request from './request'

// 维修记录接口。
export const repairApi = {
  list: (params) => request.get('/repairs', { params }),
  detail: (id) => request.get(`/repairs/${id}`),
  listByFault: (faultId) => request.get(`/repairs/fault/${faultId}`),
  create: (data) => request.post('/repairs', data),
  update: (id, data) => request.put(`/repairs/${id}`, data),
  finish: (id, data) => request.post(`/repairs/${id}/finish`, data),
  remove: (id) => request.delete(`/repairs/${id}`),
  meta: () => request.get('/repairs/meta'),
  statistics: () => request.get('/repairs/statistics'),
  // 结果更正: 单笔(原子) / 批量(atomic 整批不动, partial 逐条处理)
  correct: (id, data) => request.post(`/repairs/${id}/correct`, data),
  correctBatch: (data) => request.post('/repairs/corrections', data),
  corrections: (params) => request.get('/repairs/corrections', { params }),
  correctionsByRepair: (id) => request.get(`/repairs/${id}/corrections`),
  // 班组月度归集与结算
  aggregation: (month) => request.get('/repairs/aggregation', { params: { month } }),
  settlements: () => request.get('/repairs/settlements'),
  settle: (data) => request.post('/repairs/settlements', data),
}
