import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { createQueryClient } from '@/api/query-client'
import { App } from '@/app'
import { createAppRouter } from '@/router'

import './index.css'

const queryClient = createQueryClient()
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
