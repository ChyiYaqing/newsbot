import ArticleCard from './ArticleCard'

function SkeletonCard() {
  return (
    <div className="card skeleton">
      <div className="skeleton-line short" />
      <div className="skeleton-line" />
      <div className="skeleton-line medium" />
      <div className="skeleton-line short" />
    </div>
  )
}

export default function ArticleList({ articles, loading, error, onSelect }) {
  if (loading) {
    return (
      <div className="articles">
        {Array.from({ length: 5 }, (_, i) => <SkeletonCard key={i} />)}
      </div>
    )
  }

  if (error) {
    return <div className="error">Failed to load articles: {error}</div>
  }

  if (articles.length === 0) {
    return <div className="empty">No articles found in this time window.</div>
  }

  return (
    <div className="articles">
      {articles.map((article, i) => (
        <ArticleCard
          key={article.id}
          article={article}
          rank={i + 1}
          onSelect={onSelect}
        />
      ))}
    </div>
  )
}
