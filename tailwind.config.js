/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './templates/**/*.html',
    './static/js/**/*.js',
  ],
  theme: {
    extend: {
      colors: {
        primary: '#4F46E5',
        secondary: '#10B981',
        accent: '#F59E0B',
      },
    },
  },
  // Some classes are emitted from JavaScript template literals in ways the
  // scanner can't infer (e.g. `${cond ? 'bg-yellow-100' : ''}`). The bulk are
  // already detected because they appear as string literals inside the
  // .html files Tailwind scans, but list defensive entries here.
  // Note: dynamic classes injected via JS are still picked up because the JS
  // string literals live inside scanned `templates/**/*.html` files. Add to
  // this safelist only if a class is generated outside scanned files.
  safelist: [
    'htmx-indicator',
    'htmx-request',
  ],
  plugins: [],
};
