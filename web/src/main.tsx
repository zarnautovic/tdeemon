import { QueryClient } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { App } from '@/app'
import { createAppRouter } from '@/router'

import './index.css'

const queryClient = new QueryClient()
const router = createAppRouter({ queryClient })

const root = document.getElementById('root')
if (!root) {
  throw new Error('index.html has no #root element')
}
createRoot(root).render(
  <StrictMode>
    <App router={router} queryClient={queryClient} />
  </StrictMode>,
)
