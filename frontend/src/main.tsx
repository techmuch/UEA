import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { initializeShell } from 'nexus-shell'
import './index.css'
import App from './App.tsx'

// Initialize core shell services
initializeShell();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
