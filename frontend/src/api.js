const BASE = '/api'

export async function fetchArticles(timeWindow = '24h', limit = 50) {
  const res = await fetch(`${BASE}/articles?window=${timeWindow}&limit=${limit}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export async function fetchArticle(id) {
  const res = await fetch(`${BASE}/articles/${id}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}
