function scoreClass(score) {
  return score >= 24 ? 'high' : score >= 15 ? 'medium' : 'low'
}

function rankClass(rank) {
  return rank === 1 ? 'gold' : rank === 2 ? 'silver' : rank === 3 ? 'bronze' : ''
}

function formatDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

export default function ArticleCard({ article, rank, onSelect }) {
  const cls = scoreClass(article.total_score)
  const pct = Math.round((article.total_score / 30) * 100)
  const keywords = article.keywords
    ? article.keywords.split(',').map(k => k.trim()).filter(Boolean)
    : []

  return (
    <article className="card" onClick={() => onSelect(article.id)}>
      <div className="card-header">
        <span className={`rank ${rankClass(rank)}`}>#{rank}</span>
        <div className="score-wrap">
          <div className="score-bar">
            <div className={`score-fill ${cls}`} style={{ width: `${pct}%` }} />
          </div>
          <span className={`score-text ${cls}`}>{article.total_score}/30</span>
        </div>
        {article.category && (
          <span className="badge category">{article.category}</span>
        )}
      </div>

      <div className="card-body">
        <h2 className="title">
          <a
            href={article.url}
            target="_blank"
            rel="noopener noreferrer"
            onClick={e => e.stopPropagation()}
          >
            {article.title}
          </a>
        </h2>
        {article.title_cn && <p className="title-cn">{article.title_cn}</p>}
        {article.recommend_reason && (
          <p className="reason">{article.recommend_reason}</p>
        )}
        {keywords.length > 0 && (
          <div className="keywords">
            {keywords.map(k => <span key={k} className="keyword">{k}</span>)}
          </div>
        )}
      </div>

      <div className="card-footer">
        <span className="source">{article.source}</span>
        {article.published_at && (
          <>
            <span className="dot">·</span>
            <span className="date">{formatDate(article.published_at)}</span>
          </>
        )}
        <a
          href={article.url}
          target="_blank"
          rel="noopener noreferrer"
          className="read-link"
          onClick={e => e.stopPropagation()}
        >
          Read ↗
        </a>
      </div>
    </article>
  )
}
