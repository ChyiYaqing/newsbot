import { useState, useEffect, useCallback } from 'react'
import { fetchArticle } from '../api'

function scoreClass(score) {
  return score >= 24 ? 'high' : score >= 15 ? 'medium' : 'low'
}

function formatDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

const SCORE_DIMS = [
  { key: 'relevance', label: '相关性' },
  { key: 'quality', label: '质量' },
  { key: 'timeliness', label: '时效性' },
]

export default function ArticleModal({ id, onClose }) {
  const [article, setArticle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    setArticle(null)
    fetchArticle(id)
      .then(data => setArticle(data.article))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [id])

  const handleKeyDown = useCallback(e => {
    if (e.key === 'Escape') onClose()
  }, [onClose])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  const cls = article ? scoreClass(article.total_score) : ''
  const keywords = article?.keywords
    ? article.keywords.split(',').map(k => k.trim()).filter(Boolean)
    : []

  return (
    <div className="modal">
      <div className="modal-backdrop" onClick={onClose} />
      <div className="modal-content">
        <div className="modal-header">
          <button className="modal-close" onClick={onClose} aria-label="Close">✕</button>
        </div>
        <div className="modal-body">
          {loading && <div className="modal-loading">Loading...</div>}
          {error && <div className="error" style={{ padding: '20px 0' }}>{error}</div>}
          {article && (
            <>
              <div className="modal-badges">
                <span className={`score-text ${cls}`}>{article.total_score}/30</span>
                {article.category && (
                  <span className="badge category">{article.category}</span>
                )}
              </div>

              <h2 className="modal-title">
                <a href={article.url} target="_blank" rel="noopener noreferrer">
                  {article.title} ↗
                </a>
              </h2>
              {article.title_cn && (
                <p className="modal-title-cn">{article.title_cn}</p>
              )}

              {article.recommend_reason && (
                <div className="modal-section">
                  <h3>推荐理由</h3>
                  <p>{article.recommend_reason}</p>
                </div>
              )}

              {article.ai_summary && (
                <div className="modal-section">
                  <h3>AI 摘要</h3>
                  <p>{article.ai_summary}</p>
                </div>
              )}

              <div className="modal-section">
                <h3>评分详情</h3>
                <div className="score-detail">
                  {SCORE_DIMS.map(({ key, label }) => (
                    <div key={key} className="score-item">
                      <span className="label">{label}</span>
                      <div className="mini-bar">
                        <div style={{ width: `${article[key] * 10}%` }} />
                      </div>
                      <span className="val">{article[key]}/10</span>
                    </div>
                  ))}
                </div>
              </div>

              {keywords.length > 0 && (
                <div className="modal-section">
                  <h3>关键词</h3>
                  <div className="keywords">
                    {keywords.map(k => <span key={k} className="keyword">{k}</span>)}
                  </div>
                </div>
              )}

              <div className="modal-meta">
                <span>来源: {article.source}</span>
                {article.published_at && (
                  <span>发布: {formatDate(article.published_at)}</span>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
