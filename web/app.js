const api = (path, opts) => fetch('/api' + path, {
  headers: { 'Content-Type': 'application/json' },
  ...opts
}).then(r => r.json()).catch(e => ({ error: String(e) }));

let state = { lang: 'zh', page: 'home', settings: {} };

function t(key) { return state.lang === 'en' ? (enStrings[key] || key) : key; }

const enStrings = {
  '首页': 'Home', '设置': 'Settings', '关于': 'About', '日志': 'Logs',
  '连接': 'Connect', '已连接': 'Connected', '断开中…': 'Disconnecting…',
  '出口 IP': 'Exit IP', '国家': 'Country', '延迟': 'Latency',
  '运行日志': 'Run log', '连接记录': 'Connections', '调试信息': 'Debug',
  '关于 XEHN': 'About XEHN', '代理': 'Proxy', '路由': 'Routing',
  '混合端口': 'Mixed port', 'API 端口': 'API port', '模式': 'Mode',
  '保存': 'Save', '规则': 'Rule', '全局': 'Global', '直连': 'Direct',
  '外层': 'Outer', '内层': 'Inner', '版本': 'Version',
  '出口偏慢': 'Slow exit', '当前连接': 'Current connection',
  '更新核心': 'Update core', '断开': 'Disconnect',
  'English': '中文', '中文': 'English',
};

const pageTitles = { home: '首页', settings: '设置', about: '关于', logs: '日志' };

function renderPage() {
  const head = document.getElementById('page-head');
  const body = document.getElementById('page-body');
  head.textContent = t(pageTitles[state.page] || state.page);
  const tpl = document.getElementById('tpl-' + state.page);
  if (tpl) body.innerHTML = tpl.innerHTML;
  wirePage(state.page);
}

function wirePage(page) {
  if (page === 'home') {
    const disc = document.getElementById('connect-disc');
    if (disc) disc.onclick = () => {
      const sel = document.getElementById('proto-sel');
      const proto = sel ? sel.value : 'awg';
      api('/connect', { method: 'POST', body: JSON.stringify({ proto }) });
    };
    refreshStatus();
  } else if (page === 'settings') {
    const save = document.getElementById('save-settings');
    if (save) save.onclick = () => {
      const mp = document.getElementById('mixed-port').value;
      const ap = document.getElementById('api-port').value;
      const pm = document.getElementById('proxy-mode').value;
      api('/settings', { method: 'POST', body: JSON.stringify({ mixedPort: +mp, apiPort: +ap, proxyMode: pm }) });
    };
    loadSettings();
  } else if (page === 'about') { loadAbout(); }
  else if (page === 'logs') { loadLogs(); }
}

function refreshStatus() {
  api('/status').then(s => {
    const word = document.getElementById('state-word');
    const disc = document.getElementById('connect-disc');
    if (word) word.textContent = s.connected ? t('已连接') : t('连接');
    if (disc) disc.style.borderColor = s.connected ? 'var(--ok)' : 'var(--border)';
    const ei = document.getElementById('exit-ip'); if (ei) ei.textContent = s.exitIP || '—';
    const ec = document.getElementById('exit-country'); if (ec) ec.textContent = s.country || '—';
    const el = document.getElementById('exit-latency'); if (el) el.textContent = s.latency ? s.latency + 'ms' : '—';
    const ver = document.getElementById('ver'); if (ver) ver.textContent = s.version || '';
  });
}

function loadSettings() {
  api('/settings').then(s => {
    const mp = document.getElementById('mixed-port'); if (mp) mp.value = s.mixedPort || 7890;
    const ap = document.getElementById('api-port'); if (ap) ap.value = s.apiPort || 9090;
    const pm = document.getElementById('proxy-mode'); if (pm) pm.value = s.proxyMode || 'rule';
  });
}

function loadAbout() {
  api('/about').then(a => {
    const tbl = document.getElementById('about-table');
    if (!tbl) return;
    let html = '';
    for (const [k, v] of Object.entries(a)) {
      html += '<tr><td>' + k + '</td><td>' + v + '</td></tr>';
    }
    tbl.innerHTML = html;
  });
}

function loadLogs() {
  api('/logs').then(l => {
    const view = document.getElementById('log-view');
    if (!view) return;
    view.innerHTML = (l.lines || []).map(e => '<div class="log-line ' + e.level + '">' + e.msg + '</div>').join('');
  });
}

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('#nav button').forEach(btn => {
    btn.onclick = () => {
      document.querySelectorAll('#nav button').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.page = btn.dataset.page;
      renderPage();
    };
  });
  const langBtn = document.getElementById('lang-btn');
  if (langBtn) langBtn.onclick = () => {
    state.lang = state.lang === 'zh' ? 'en' : 'zh';
    langBtn.textContent = state.lang === 'zh' ? 'English' : '中文';
    renderPage();
  };
  renderPage();
  setInterval(refreshStatus, 3000);
});
