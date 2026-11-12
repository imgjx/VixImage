// VixImage 会话自愈脚本
// 背景：部分浏览器 / 反向代理环境下 Cookie 可能不被保存或不随请求发送，
// 导致「登录成功 → 校验登录态失败 → 又被弹回登录页」的循环。
// 这里用三级兜底维持登录态：
//   1) 正常 Cookie；
//   2) localStorage 中的令牌，通过 X-Vix-Sid 请求头随所有 API 请求发送；
//   3) 站内链接自动追加 t=<令牌>，保证页面导航在无 Cookie 时依然能通过校验。
(function () {
  var KEY = 'vix_sid';

  function getTok() {
    try { return localStorage.getItem(KEY) || ''; } catch (e) { return ''; }
  }
  function setTok(t) {
    try {
      if (t) localStorage.setItem(KEY, t);
      else localStorage.removeItem(KEY);
    } catch (e) { /* 隐私模式等场景忽略 */ }
  }

  window.VixSession = {
    get: getTok,
    set: setTok,
    clear: function () { setTok(''); }
  };

  // 给站内链接补上令牌，导航不再依赖 Cookie。
  function patchLinks() {
    var t = getTok();
    if (!t) return;
    var links = document.querySelectorAll('a[href^="/"]');
    for (var i = 0; i < links.length; i++) {
      var a = links[i];
      var href = a.getAttribute('href') || '';
      if (!href || href.charAt(0) === '#') continue;
      if (href.indexOf('t=') >= 0) continue;
      // 保持相对路径，避免子路径部署时被改写成根路径。
      a.setAttribute('href', href + (href.indexOf('?') >= 0 ? '&' : '?') + 't=' + encodeURIComponent(t));
    }
  }

  // 所有 fetch 请求自动带上令牌头（老浏览器没有 fetch 时跳过，不报错）。
  if (window.fetch && window.Headers) {
    var rawFetch = window.fetch;
    window.fetch = function (input, init) {
      var t = getTok();
      if (t) {
        init = init || {};
        try {
          var h = new Headers(init.headers || {});
          if (!h.has('X-Vix-Sid')) h.set('X-Vix-Sid', t);
          init.headers = h;
        } catch (e) { /* 忽略 */ }
      }
      return rawFetch(input, init);
    };
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', patchLinks);
  } else {
    patchLinks();
  }
})();
