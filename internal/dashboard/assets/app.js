/* ---------------------------------------------------------------------------
   app.js — utilidades compartilhadas pelas páginas do dashboard.

   Autenticação: o ROOT_TOKEN é colado uma vez pelo usuário e guardado no
   sessionStorage (sobrevive à navegação entre as páginas do dashboard, some
   ao fechar a aba). Toda chamada de dados vai para as rotas protegidas
   (/admin/*, /api/status, /api/play/init) com `Authorization: Bearer`.
   --------------------------------------------------------------------------- */
(function () {
  "use strict";

  var TOKEN_KEY = "streamedia_root_token";

  // --- Token ----------------------------------------------------------------
  function getToken() { return sessionStorage.getItem(TOKEN_KEY) || ""; }
  function setToken(t) { sessionStorage.setItem(TOKEN_KEY, t); }

  // setupToken liga a barra de token (inputs #rootToken e botão #saveToken,
  // status em #tokenStatus) e chama reload() quando há um token disponível.
  function setupToken(reload) {
    var input = document.getElementById("rootToken");
    var btn = document.getElementById("saveToken");
    var status = document.getElementById("tokenStatus");
    if (input) input.value = getToken();

    function apply() {
      var t = input ? input.value.trim() : "";
      if (!t) { if (status) { status.textContent = "Informe o ROOT_TOKEN."; status.className = "note err"; } return; }
      setToken(t);
      if (status) { status.textContent = "Token salvo nesta aba."; status.className = "note"; }
      reload();
    }
    if (btn) btn.addEventListener("click", apply);
    if (input) input.addEventListener("keydown", function (e) { if (e.key === "Enter") apply(); });

    // Se já há token (vindo de outra página do dashboard), carrega de imediato.
    if (getToken()) reload();
    else if (status) { status.textContent = "Cole o ROOT_TOKEN para carregar os dados."; status.className = "note"; }
  }

  // ApiError carrega o status HTTP para o chamador decidir a mensagem.
  function ApiError(status, message) { this.status = status; this.message = message; }
  ApiError.prototype = Object.create(Error.prototype);

  // apiFetch faz uma chamada autenticada e devolve o `data` do envelope padrão
  // ({error, message, data, status_code}); lança ApiError em falha.
  async function apiFetch(pathQuery, opts) {
    opts = opts || {};
    var headers = Object.assign({}, opts.headers || {}, { "Authorization": "Bearer " + getToken() });
    var res = await fetch(pathQuery, Object.assign({}, opts, { headers: headers }));
    var body = null;
    try { body = await res.json(); } catch (e) { /* resposta sem corpo JSON */ }
    if (res.status === 401) throw new ApiError(401, "Não autorizado — verifique o ROOT_TOKEN.");
    if (!res.ok || (body && body.error)) {
      throw new ApiError(res.status, (body && body.message) || ("Erro HTTP " + res.status));
    }
    return body ? body.data : null;
  }

  // --- Formatadores ---------------------------------------------------------
  var WEEKDAYS = ["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"];
  var STATUS_LABELS = {
    pending_upload: "Aguardando upload",
    uploading: "Enviando",
    upload_complete: "Upload concluído",
    transcoding: "Processando",
    ready: "Pronto",
    failed_upload: "Falha no upload",
    failed_transcode: "Falha no processamento",
  };

  function fmtBytes(n) {
    n = Number(n) || 0;
    if (n < 1024) return n + " B";
    var units = ["KB", "MB", "GB", "TB"], i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return n.toFixed(n < 10 ? 1 : 0) + " " + units[i];
  }

  function fmtDuration(s) {
    s = Math.max(0, Math.floor(Number(s) || 0));
    var h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
    function p(x) { return (x < 10 ? "0" : "") + x; }
    return h > 0 ? (h + ":" + p(m) + ":" + p(sec)) : (p(m) + ":" + p(sec));
  }

  function fmtDateTime(iso) {
    if (!iso) return "—";
    var d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString("pt-BR");
  }

  function statusLabel(s) { return STATUS_LABELS[s] || s; }

  // escapeHtml protege contra HTML embutido em valores vindos da API ao montar
  // tabelas via innerHTML.
  function escapeHtml(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  // --- Gráficos (Chart.js) --------------------------------------------------
  // Aplica o tema escuro aos defaults do Chart.js, se a lib estiver carregada.
  function applyChartTheme() {
    if (typeof Chart === "undefined") return;
    Chart.defaults.color = "#8b98a9";
    Chart.defaults.borderColor = "#1e2533";
    Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
  }

  // mapToSeries converte um objeto {chave: contagem} em {labels, data} para um
  // domínio fixo de chaves (ex. 0..23 para horas, 0..6 para dias).
  function fixedDomainSeries(obj, keys, labelFn) {
    obj = obj || {};
    return {
      labels: keys.map(labelFn),
      data: keys.map(function (k) { return Number(obj[k] || obj[String(k)] || 0); }),
    };
  }

  function hourSeries(obj) {
    var keys = []; for (var i = 0; i < 24; i++) keys.push(i);
    return fixedDomainSeries(obj, keys, function (h) { return (h < 10 ? "0" : "") + h + "h"; });
  }

  function weekdaySeries(obj) {
    var keys = [0, 1, 2, 3, 4, 5, 6];
    return fixedDomainSeries(obj, keys, function (d) { return WEEKDAYS[d]; });
  }

  // dateSeries ordena as chaves YYYY-MM-DD e devolve labels/data.
  function dateSeries(obj) {
    obj = obj || {};
    var keys = Object.keys(obj).sort();
    return { labels: keys, data: keys.map(function (k) { return Number(obj[k] || 0); }) };
  }

  // barChart cria (e devolve) um gráfico de barras no canvas indicado, com a
  // cor de acento do tema. Destrói um gráfico anterior preso ao mesmo canvas.
  var _charts = {};
  function barChart(canvasId, series, label, color) {
    if (typeof Chart === "undefined") return null;
    var el = document.getElementById(canvasId);
    if (!el) return null;
    if (_charts[canvasId]) _charts[canvasId].destroy();
    _charts[canvasId] = new Chart(el, {
      type: "bar",
      data: {
        labels: series.labels,
        datasets: [{ label: label, data: series.data, backgroundColor: color || "#6ea8fe", borderRadius: 3 }],
      },
      options: {
        responsive: true,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
      },
    });
    return _charts[canvasId];
  }

  function doughnutChart(canvasId, labels, data) {
    if (typeof Chart === "undefined") return null;
    var el = document.getElementById(canvasId);
    if (!el) return null;
    if (_charts[canvasId]) _charts[canvasId].destroy();
    var palette = ["#6ea8fe", "#4ade80", "#fbbf24", "#f87171", "#a78bfa", "#22d3ee", "#94a3b8"];
    _charts[canvasId] = new Chart(el, {
      type: "doughnut",
      data: { labels: labels, datasets: [{ data: data, backgroundColor: palette, borderColor: "#11151f", borderWidth: 2 }] },
      options: { responsive: true, plugins: { legend: { position: "bottom" } } },
    });
    return _charts[canvasId];
  }

  // qs lê um parâmetro da query string da página atual.
  function qs(name, def) {
    var v = new URLSearchParams(window.location.search).get(name);
    return v == null ? def : v;
  }

  // Exporta no escopo global um único namespace para as páginas usarem.
  window.Dash = {
    getToken: getToken, setToken: setToken, setupToken: setupToken,
    apiFetch: apiFetch,
    fmtBytes: fmtBytes, fmtDuration: fmtDuration, fmtDateTime: fmtDateTime,
    statusLabel: statusLabel, escapeHtml: escapeHtml, WEEKDAYS: WEEKDAYS,
    applyChartTheme: applyChartTheme, barChart: barChart, doughnutChart: doughnutChart,
    hourSeries: hourSeries, weekdaySeries: weekdaySeries, dateSeries: dateSeries,
    qs: qs,
  };
})();
