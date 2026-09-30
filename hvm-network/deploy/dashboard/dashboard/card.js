// HVM_NETWORK_CARD_V3
(() => {
  'use strict';
  const root = document.getElementById('view-hvm');
  const el = id => document.getElementById('hvm-' + id);
  let lang = 'it', previous = null, sample = null, progress = 'first', busy = false, error = '', rpcOK = false, sampledAt = '';
  try { lang = localStorage.getItem('hvm-language') === 'en' ? 'en' : 'it'; } catch (_) {}
  const words = {
    it: {unknown:'Non verificato', first:'Primo campione; aggiorna per confrontare', advanced:'Avanzata dal campione precedente', same:'Nessun avanzamento dal campione precedente', decreased:'Altezza diminuita: verificare', responding:'Observer rispondente', stopped:'Reactor fermo', stale:'Stato dettagliato non disponibile', unavailable:'Non disponibile', running:'In esecuzione', pending:'In attesa', activated:'Altezza di attivazione finalizzata', rpc:'RPC testnet rispondente', time:'Ultimo campione'},
    en: {unknown:'Not checked', first:'First sample; refresh to compare', advanced:'Advanced since previous sample', same:'No advancement since previous sample', decreased:'Height decreased: investigate', responding:'Observer responding', stopped:'Reactor stopped', stale:'Detailed status unavailable', unavailable:'Unavailable', running:'Running', pending:'Pending', activated:'Activation height finalized', rpc:'Testnet RPC responding', time:'Last sample'}
  };
  function render() {
    root.lang = lang;
    root.querySelectorAll('[data-hvm-it]').forEach(n => { n.textContent = n.getAttribute('data-hvm-' + lang); });
    el('it').setAttribute('aria-pressed', String(lang === 'it')); el('en').setAttribute('aria-pressed', String(lang === 'en'));
    const t = words[lang];
    el('status').textContent = error ? t.unavailable + ': ' + error : sample ? t.responding : t.unknown;
    el('time').textContent = sampledAt ? t.time + ': ' + sampledAt : '';
    for (const id of ['finalized','progress','peers','reactor','activation','rpc']) el(id).textContent = error ? t.unavailable : t.unknown;
    if (!sample || error) return;
    el('finalized').textContent = String(sample.finalized_height);
    el('peers').textContent = String(sample.peer_count);
    el('progress').textContent = t[progress];
    el('reactor').textContent = sample.reactor_status_fresh && sample.reactor ? (sample.reactor_running ? t.running : t.stopped) + ' / ' + sample.reactor.height + ' / ' + sample.reactor.round + ' / ' + sample.reactor.step : t.stale;
    el('activation').textContent = sample.evm_activation_height ? (sample.finalized_height >= sample.evm_activation_height ? t.activated : t.pending) + ' (' + sample.evm_activation_height + ')' : t.unknown;
    el('rpc').textContent = rpcOK ? t.rpc : t.unavailable;
  }
  async function refresh() {
    if (busy) return;
    busy = true; el('refresh').disabled = true;
    try {
      const r = await fetch('/api/hashburst/hvm/testnet/network', {cache:'no-store', signal:AbortSignal.timeout(15000)});
      if (!r.ok) throw Error('HTTP ' + r.status);
      const d = await r.json();
      if (d.ok !== true || d.chain_id !== 4735490 || d.role !== 'observer' || d.node_id !== 'hvm-testnet-ingress' || !Number.isSafeInteger(d.finalized_height) || d.finalized_height < 0) throw Error('network/identity');
      progress = previous === null ? 'first' : d.finalized_height > previous ? 'advanced' : d.finalized_height === previous ? 'same' : 'decreased';
      previous = d.finalized_height; sample = d; error = ''; sampledAt = new Date().toISOString(); rpcOK = false;
      try {
        const rr = await fetch('/api/hashburst/hvm/testnet/evm', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({jsonrpc:'2.0',id:1,method:'eth_chainId',params:[]}), signal:AbortSignal.timeout(8000)});
        const v = rr.ok ? await rr.json() : null;
        rpcOK = !!v && !v.error && v.result === '0x484202';
      } catch (_) {}
    } catch (e) { error = e.message; sample = null; previous = null; }
    finally { busy = false; el('refresh').disabled = false; render(); }
  }
  for (const value of ['it','en']) el(value).addEventListener('click', () => {
    lang = value; try { localStorage.setItem('hvm-language', lang); } catch (_) {} render();
  });
  document.getElementById('nav-hvm').addEventListener('click', refresh);
  el('refresh').addEventListener('click', refresh);
  setInterval(() => { if (root.classList.contains('active') && !document.hidden) refresh(); }, 30000);
  render();
})();
