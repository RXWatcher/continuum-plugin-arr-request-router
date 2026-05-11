import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import App from './App';
import './index.css';
import { mountPath } from './lib/mountPath';
import { captureFromURL, getCachedTheme } from './lib/auth';

// Capture token + theme from the initial URL (sidebar link click sets these).
const params = new URLSearchParams(window.location.search);
captureFromURL(params);

// Strip ?token= from the URL so it does not show in browser history.
if (params.has('token')) {
  params.delete('token');
  const cleaned = params.toString();
  const url = window.location.pathname + (cleaned ? `?${cleaned}` : '') + window.location.hash;
  window.history.replaceState(null, '', url);
}

// Apply continuum's theme to the plugin's <html> so semantic Tailwind classes
// inherit continuum's palette.
const theme = getCachedTheme();
if (theme) {
  document.documentElement.dataset.theme = theme;
}

const basename = `${mountPath()}/admin`;

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter basename={basename}>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
