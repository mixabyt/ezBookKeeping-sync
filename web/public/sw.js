// Мінімальний service worker: кешуємо лише оболонку застосунку.
// Дані (/api/*) НЕ кешуються навмисно — краще показати помилку мережі,
// ніж старий список категорій, за яким людина створить неправильне правило.
const CACHE = 'ezbk-shell-v2';
const SHELL = ['/', '/vue.global.prod.js', '/icon.svg', '/manifest.webmanifest'];

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(SHELL)));
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((names) => Promise.all(names.filter((n) => n !== CACHE).map((n) => caches.delete(n))))
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== 'GET' || url.pathname.startsWith('/api/')) return;
  event.respondWith(
    fetch(event.request).catch(() => caches.match(event.request).then((hit) => hit || caches.match('/')))
  );
});
