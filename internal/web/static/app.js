/* my-go-exchange market terminal */
(() => {
  const state = {
    selected: null,
    quotes: new Map(), // id -> {tick, prevPrice}
    tape: [],
    klines: {},
    total: 0,
    meta: {},
    ws: null,
  };

  const $ = (id) => document.getElementById(id);
  const fmt = (v, d = 2) => {
    if (v === null || v === undefined || Number.isNaN(v)) return "—";
    return Number(v).toLocaleString("zh-CN", {
      minimumFractionDigits: d,
      maximumFractionDigits: d,
    });
  };
  const fmtInt = (v) => (v === null || v === undefined ? "—" : Number(v).toLocaleString("zh-CN"));
  const digitsFor = (id) => (/au|ag/i.test(id) ? 2 : /rb|al|m|i|SR|TA/i.test(id) ? 0 : 0);

  function chgInfo(tick) {
    const last = tick.price;
    const ref = tick.pre_settlement ?? tick.pre_close;
    if (last == null || ref == null || ref === 0) return { abs: null, pct: null, cls: "flat" };
    const abs = last - ref;
    const pct = (abs / ref) * 100;
    const cls = abs > 0 ? "up" : abs < 0 ? "down" : "flat";
    return { abs, pct, cls };
  }

  function connectWS() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/ws`);
    state.ws = ws;
    setConn("connecting");

    ws.onopen = () => setConn("ok");
    ws.onclose = () => {
      setConn("bad");
      setTimeout(connectWS, 1500);
    };
    ws.onerror = () => setConn("bad");
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      if (msg.type === "snapshot") applySnapshot(msg.data);
      else if (msg.type === "tick") applyTick(msg.data);
      else if (msg.type === "kline") applyKline(msg.data);
    };
  }

  function setConn(mode) {
    const dot = $("wsDot");
    const text = $("wsText");
    dot.classList.remove("ok", "bad");
    if (mode === "ok") {
      dot.classList.add("ok");
      text.textContent = "实时已连接";
    } else if (mode === "bad") {
      dot.classList.add("bad");
      text.textContent = "重连中…";
    } else {
      text.textContent = "连接中…";
    }
  }

  function applySnapshot(data) {
    state.meta = data.meta || {};
    state.total = data.total || 0;
    state.klines = data.klines || {};
    state.tape = data.tape || [];
    state.quotes.clear();
    (data.quotes || []).forEach((t) => {
      state.quotes.set(t.instrument_id, { tick: t, prev: t.price });
    });
    if (!state.selected && state.quotes.size) {
      state.selected = [...state.quotes.keys()][0];
    }
    $("frontText").textContent = state.meta.front || "—";
    $("streamText").textContent = state.meta.stream || "MD_TICKS";
    renderAll();
  }

  function applyTick(tick) {
    const id = tick.instrument_id;
    const prev = state.quotes.get(id);
    state.quotes.set(id, { tick, prev: prev ? prev.tick.price : tick.price });
    state.total += 1;
    state.tape.push({
      instrument_id: id,
      price: tick.price,
      volume: tick.volume,
      bid: tick.bid,
      ask: tick.ask,
      update_time: tick.update_time,
      timestamp: tick.timestamp,
    });
    if (state.tape.length > 200) state.tape = state.tape.slice(-200);
    renderRow(id, true);
    if (id === state.selected) renderDetail();
    renderTape();
    $("totalTicks").textContent = `ticks: ${state.total}`;
  }

  function applyKline(data) {
    const id = data.instrument_id;
    const bar = data.bar;
    if (!id || !bar) return;
    const bars = state.klines[id] || [];
    if (bars.length && bars[bars.length - 1].ts === bar.ts) {
      bars[bars.length - 1] = bar;
    } else {
      bars.push(bar);
      if (bars.length > 240) bars.shift();
    }
    state.klines[id] = bars;
    if (id === state.selected) drawKline();
  }

  function renderAll() {
    const body = $("quoteBody");
    body.innerHTML = "";
    const ids = [...state.quotes.keys()].sort();
    ids.forEach((id) => {
      const tr = document.createElement("tr");
      tr.dataset.id = id;
      tr.addEventListener("click", () => {
        state.selected = id;
        renderAll();
      });
      body.appendChild(tr);
      renderRow(id, false);
    });
    $("quoteCount").textContent = String(state.quotes.size);
    renderDetail();
    renderTape();
    drawKline();
    $("totalTicks").textContent = `ticks: ${state.total}`;
  }

  function renderRow(id, animate) {
    let tr = document.querySelector(`#quoteBody tr[data-id="${id}"]`);
    if (!tr) {
      renderAll();
      return;
    }
    const entry = state.quotes.get(id);
    if (!entry) return;
    const t = entry.tick;
    const ch = chgInfo(t);
    const dig = digitsFor(id);
    const lastCls = ch.cls;
    const flashCls = animate ? (ch.cls === "down" ? "flash-down" : "flash-up") : "";

    tr.classList.toggle("active", id === state.selected);
    tr.innerHTML = `
      <td class="sym">${id}</td>
      <td class="${lastCls}">${fmt(t.price, dig)}</td>
      <td class="${ch.cls}">${ch.abs == null ? "—" : (ch.abs > 0 ? "+" : "") + fmt(ch.abs, dig) + " / " + (ch.pct > 0 ? "+" : "") + fmt(ch.pct, 2) + "%"}</td>
      <td class="up">${fmt(t.bid, dig)}</td>
      <td class="down">${fmt(t.ask, dig)}</td>
      <td>${fmtInt(t.volume)}</td>
    `;
    if (flashCls) {
      tr.classList.remove("flash-up", "flash-down");
      // reflow
      void tr.offsetWidth;
      tr.classList.add(flashCls);
    }
  }

  function renderDetail() {
    const id = state.selected;
    const entry = id && state.quotes.get(id);
    $("detailSymbol").textContent = id || "选择合约";
    if (!entry) {
      $("detailLast").textContent = "—";
      $("detailChg").textContent = "";
      $("detailGrid").innerHTML = "";
      return;
    }
    const t = entry.tick;
    const ch = chgInfo(t);
    const dig = digitsFor(id);
    $("detailLast").textContent = fmt(t.price, dig);
    $("detailLast").className = `last ${ch.cls}`;
    $("detailChg").textContent = ch.abs == null ? "" : `${ch.abs > 0 ? "+" : ""}${fmt(ch.abs, dig)} (${ch.pct > 0 ? "+" : ""}${fmt(ch.pct, 2)}%)`;
    $("detailChg").className = `chg ${ch.cls}`;

    const cells = [
      ["买一 / 量", `${fmt(t.bid, dig)} / ${fmtInt(t.bid_volume)}`],
      ["卖一 / 量", `${fmt(t.ask, dig)} / ${fmtInt(t.ask_volume)}`],
      ["成交量", fmtInt(t.volume)],
      ["持仓量", fmtInt(t.open_interest)],
      ["今开", fmt(t.open, dig)],
      ["最高", fmt(t.high, dig)],
      ["最低", fmt(t.low, dig)],
      ["昨收", fmt(t.pre_close, dig)],
      ["昨结", fmt(t.pre_settlement, dig)],
      ["涨停", fmt(t.upper_limit, dig)],
      ["跌停", fmt(t.lower_limit, dig)],
      ["更新时间", `${t.update_time || "—"}.${String(t.update_millisec || 0).padStart(3, "0")}`],
    ];
    $("detailGrid").innerHTML = cells
      .map(([k, v]) => `<div class="kv"><div class="k">${k}</div><div class="v">${v}</div></div>`)
      .join("");
  }

  function renderTape() {
    const el = $("tape");
    const rows = state.tape.slice(-80).reverse();
    el.innerHTML = rows
      .map((item) => {
        const dig = digitsFor(item.instrument_id);
        const cls = item.price != null ? "up" : "flat";
        return `<div class="tape-row">
          <span class="sym">${item.instrument_id}</span>
          <span class="${cls}">${fmt(item.price, dig)}</span>
          <span class="flat">${item.update_time || ""}</span>
        </div>`;
      })
      .join("");
  }

  function drawKline() {
    const canvas = $("kline");
    const id = state.selected;
    const bars = (id && state.klines[id]) || [];
    const ctx = canvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    const cssW = canvas.clientWidth || 900;
    const cssH = canvas.clientHeight || 280;
    canvas.width = Math.floor(cssW * dpr);
    canvas.height = Math.floor(cssH * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);

    // padding
    const padL = 8, padR = 56, padT = 12, padB = 22;
    const w = cssW - padL - padR;
    const h = cssH - padT - padB;

    if (!bars.length) {
      ctx.fillStyle = "#8b9bb4";
      ctx.font = "12px sans-serif";
      ctx.fillText("暂无 K 线数据", padL + 8, padT + 20);
      return;
    }

    let min = Infinity, max = -Infinity;
    bars.forEach((b) => {
      min = Math.min(min, b.low);
      max = Math.max(max, b.high);
    });
    if (min === max) {
      min -= 1;
      max += 1;
    }
    const span = max - min;
    const y = (v) => padT + (1 - (v - min) / span) * h;
    const n = bars.length;
    const slot = w / n;
    const cw = Math.max(2, Math.min(10, slot * 0.62));

    // grid
    ctx.strokeStyle = "rgba(34,48,71,0.8)";
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const yy = padT + (h * i) / 4;
      ctx.beginPath();
      ctx.moveTo(padL, yy);
      ctx.lineTo(padL + w, yy);
      ctx.stroke();
      const val = max - (span * i) / 4;
      ctx.fillStyle = "#8b9bb4";
      ctx.font = "11px JetBrains Mono, monospace";
      ctx.fillText(val.toFixed(span > 50 ? 0 : 2), padL + w + 8, yy + 3);
    }

    bars.forEach((b, i) => {
      const cx = padL + i * slot + slot / 2;
      const up = b.close >= b.open;
      const color = up ? "#3ddc97" : "#ff5c5c";
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.beginPath();
      ctx.moveTo(cx, y(b.high));
      ctx.lineTo(cx, y(b.low));
      ctx.stroke();
      const top = y(Math.max(b.open, b.close));
      const bot = y(Math.min(b.open, b.close));
      const bh = Math.max(1, bot - top);
      ctx.fillRect(cx - cw / 2, top, cw, bh);
    });

    // last price line
    const last = bars[bars.length - 1].close;
    ctx.strokeStyle = "rgba(91,157,255,0.7)";
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    ctx.moveTo(padL, y(last));
    ctx.lineTo(padL + w, y(last));
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = "#5b9dff";
    ctx.font = "11px JetBrains Mono, monospace";
    ctx.fillText(last.toFixed(span > 50 ? 0 : 2), padL + w + 8, y(last) + 3);
  }

  // clock
  setInterval(() => {
    $("clock").textContent = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  }, 500);

  window.addEventListener("resize", () => drawKline());
  connectWS();
})();
