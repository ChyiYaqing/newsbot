import { useState, useEffect } from 'react'
import Header from './components/Header'
import ArticleList from './components/ArticleList'
import ArticleModal from './components/ArticleModal'
import { fetchArticles } from './api'

const WINDOWS = ['24h', '3days', '7days']

function formatWindow(w) {
  return { '24h': '24 hours', '3days': '3 days', '7days': '7 days' }[w] || w
}

export default function App() {
  const [timeWindow, setTimeWindow] = useState(() => {
    const w = new URLSearchParams(location.search).get('window')
    return WINDOWS.includes(w) ? w : '24h'
  })
  const [articles, setArticles] = useState([])
  const [count, setCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [selectedId, setSelectedId] = useState(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    fetchArticles(timeWindow)
      .then(data => {
        setArticles(data.articles ?? [])
        setCount(data.count ?? 0)
      })
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [timeWindow])

  function handleWindowChange(win) {
    setTimeWindow(win)
    history.pushState({}, '', `?window=${win}`)
  }

  return (
    <>
      <Header currentWindow={timeWindow} onWindowChange={handleWindowChange} />
      <main className="main">
        <div className="container">
          {!loading && !error && (
            <p className="stats">{count} articles · last {formatWindow(timeWindow)}</p>
          )}
          <ArticleList
            articles={articles}
            loading={loading}
            error={error}
            onSelect={setSelectedId}
          />
        </div>
      </main>
      {selectedId && (
        <ArticleModal id={selectedId} onClose={() => setSelectedId(null)} />
      )}
    </>
  )
}
