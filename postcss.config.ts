// PostCSS pipeline: Tailwind as a plugin plus autoprefixer for the XP
// floor, minified with cssnano in production builds. The browserslist
// entry in package.json drives autoprefixer.

export default {
  plugins: {
    'postcss-import': {},
    tailwindcss: {},
    'postcss-sort-media-queries': {},
    'postcss-combine-media-query': {},
    autoprefixer: {},
    ...(process.env.NODE_ENV === 'production' ? { cssnano: {} } : {}),
  },
}
