import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/fonts'
import './styles/tokens.css'
import './styles/mockup.css'
import './styles/app.css'
import App from './App'
import { applyAppearance } from './app/appearance'

applyAppearance() // before the first paint: light unless the user chose otherwise

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
