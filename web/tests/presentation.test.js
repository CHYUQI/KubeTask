import test, { before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'vite';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
let server, Charts, trendGeometry, typeSegments, highlightLog, logSearchPattern, lineClass;
before(async () => {
  server = await createServer({ optimizeDeps: { noDiscovery: true, include: [] },
    server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
  ({ default: Charts, trendGeometry, typeSegments } = await server.ssrLoadModule('/src/components/DashboardCharts.vue'));
  ({ highlightLog, logSearchPattern, lineClass } = await server.ssrLoadModule('/src/pages/TaskLogs.vue'));
});
test('log search highlights literal characters before HTML escaping, without injecting markup', () => {
  assert.equal(highlightLog('<img src=x> & done', logSearchPattern('<img')),
    '<mark>&lt;img</mark> src=x&gt; &amp; done');
  assert.equal(highlightLog('a & b', logSearchPattern('&')), 'a <mark>&amp;</mark> b');
  assert.equal(highlightLog('a.b A.B', logSearchPattern('a.b')), '<mark>a.b</mark> <mark>A.B</mark>');
  assert.equal(highlightLog('a+b [x]', logSearchPattern('[x]')), 'a+b <mark>[x]</mark>');
  assert.equal(highlightLog('<script>bad()</script>', null), '&lt;script&gt;bad()&lt;/script&gt;');
});
test('log severity preserves case-insensitive precedence', () => {
  assert.equal(lineClass('WARN followed by FAILURE'), 'level-error');
  assert.equal(lineClass('Warning: retry'), 'level-warn');
  assert.equal(lineClass('INFO complete'), '');
});
after(async () => { await server?.close(); });
test('trend sorts without mutation and handles zero, single-day, narrow and large counts', () => {
  for (const data of [[], [{ date: '2026-10-22', succeeded: 0, failed: 0 }],
    [{ date: '2026-10-22', succeeded: 15000, failed: 5 }, { date: '2026-10-21', succeeded: 3, failed: 1 }]]) {
    const snapshot = structuredClone(data);
    const plot = trendGeometry(data, 180);
    assert.deepEqual(data, snapshot);
    assert.ok(!/NaN|Infinity/.test(plot.successPath + plot.failurePath));
    assert.ok(plot.ticks.every(tick => Number.isInteger(tick.value)));
    for (const point of plot.points) {
      assert.ok(point.x >= plot.left && point.x <= plot.right);
      assert.ok(point.successY >= 14 && point.successY <= 190);
      assert.ok(point.failureY >= 14 && point.failureY <= 190);
    }
    assert.deepEqual(plot.points.map(point => point.date), [...data.map(point => point.date)].sort());
  }
});
test('donut handles empty and full-circle data, with proportional non-overlapping segments', () => {
  assert.deepEqual(typeSegments({ onetime: 0, cron: 0, delay: 0 }), []);
  assert.equal(typeSegments({ onetime: 8 })[0].length, 100);
  const segments = typeSegments({ onetime: 4, cron: 3, delay: 1 });
  assert.equal(segments.reduce((sum, segment) => sum + segment.percent, 0), 100);
  segments.forEach((segment, index) => {
    assert.ok(segment.length <= segment.percent);
    if (index) assert.ok(segment.offset >= segments[index - 1].offset + segments[index - 1].length);
  });
});
test('SSR renders responsive SVG charts with keyboard data points, labels and empty states', async () => {
  const html = await renderToString(createSSRApp(Charts, {
    loaded: true, stats: { total: 8, onetime: 4, cron: 3, delay: 1 },
    trend: [{ date: '2026-10-22', succeeded: 7, failed: 1 }],
  }));
  assert.equal((html.match(/<svg /g) || []).length, 2);
  assert.ok(html.includes('2026-10-22，成功 7，失败 1'));
  assert.ok(html.includes('OneTime：4 个，50%'));
  assert.ok(html.includes('aria-pressed="true"'));
  assert.equal((html.match(/stroke-linecap="round"/g) || []).length, 3);
  assert.ok(!/NaN|Infinity/.test(html));
  const empty = await renderToString(createSSRApp(Charts, { loaded: true, stats: { total: 0 }, trend: [] }));
  assert.ok(empty.includes('暂无执行记录'));
});
