// HVM_NETWORK_CARD_V1
let hvmPrevious = null;
let hvmBusy = false;
async function loadHVMNetwork() {
  if (hvmBusy) return;
  hvmBusy = true;
  $('hvm-refresh').disabled = true;
  try {
    const res = await fetch('/api/hashburst/hvm/testnet/network', {cache: 'no-store', signal: AbortSignal.timeout(15000)});
    if (!res.ok) throw new Error('Observer HTTP ' + res.status);
    const d = await res.json();
    if (d.chain_id !== 4735490 || d.role !== 'observer' || !Number.isSafeInteger(d.finalized_height)) throw new Error('Unexpected network response');
    $('hvm-chain').textContent = String(d.chain_id);
    $('hvm-finalized').textContent = String(d.finalized_height);
    $('hvm-peers').textContent = String(d.peer_count);
    $('hvm-reactor').textContent = d.reactor_status_fresh ? `${d.reactor_running ? 'Running' : 'Stopped'}; height ${d.reactor.height}, round ${d.reactor.round}, ${d.reactor.step}` : 'Detailed reactor status temporarily unavailable';
    $('hvm-status').textContent = d.reactor_running ? 'Observer responding; verify progress below' : 'Observer responding; reactor stopped';
    $('hvm-progress').textContent = hvmPrevious === null ? 'First sample; refresh to compare' : d.finalized_height > hvmPrevious ? 'Advanced since previous sample' : d.finalized_height === hvmPrevious ? 'No advancement since previous sample' : 'Height decreased — investigate';
    hvmPrevious = d.finalized_height;
    $('hvm-time').textContent = new Date().toISOString();
    $('hvm-activation').textContent = d.evm_activation_height ? `${d.finalized_height >= d.evm_activation_height ? 'Activation height finalized' : 'Pending'} (${d.evm_activation_height})` : 'Activation configuration unavailable';
    try {
      const r = await fetch('/api/hashburst/hvm/testnet/evm', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({jsonrpc:'2.0',id:1,method:'eth_chainId',params:[]}), signal:AbortSignal.timeout(8000)});
      const v = r.ok ? await r.json() : null;
      $('hvm-rpc').textContent = v && v.result === '0x484202' ? 'Responding on testnet; wallet tests remain separate' : 'Unavailable or not yet published';
    } catch (_) { $('hvm-rpc').textContent = 'Unavailable or not yet published'; }
  } catch (e) {
    $('hvm-status').textContent = 'Unavailable: ' + e.message;
    for (const id of ['hvm-finalized','hvm-progress','hvm-peers','hvm-reactor','hvm-activation','hvm-rpc']) $(id).textContent = 'Unavailable — previous sample is stale';
    hvmPrevious = null;
  } finally { hvmBusy = false; $('hvm-refresh').disabled = false; }
}
$('nav-hvm').addEventListener('click', loadHVMNetwork);
$('hvm-refresh').addEventListener('click', loadHVMNetwork);
setInterval(() => { if (state.view === 'hvm' && !document.hidden) loadHVMNetwork(); }, 30000);
