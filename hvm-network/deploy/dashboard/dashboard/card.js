// HVM_NETWORK_CARD_V5_2_EN
(() => {
  'use strict';
  const root=document.getElementById('view-hvm'), el=id=>document.getElementById('hvm-'+id);
  const networks={
    mainnet:{chain:4735489,hex:'0x484201',name:'HashBurst HVM Mainnet',node:'hvm-mainnet-observer-01',network:'/api/hashburst/hvm/mainnet/network',rpc:'/api/hashburst/hvm/mainnet/evm',rpcPublic:'https://blockchainapi.one/api/hashburst/hvm/mainnet/evm',ws:'wss://blockchainapi.one/api/hashburst/hvm/mainnet/evm/ws',federation:'4 / 4 — R9 production acceptance',apow:'Active — production service; rewards require BFT finality',note:'Mainnet production acceptance R9 completed on 12 November 2026.'},
    testnet:{chain:4735490,hex:'0x484202',name:'HashBurst HVM Testnet',node:'hvm-testnet-ingress',network:'/api/hashburst/hvm/testnet/network',rpc:'/api/hashburst/hvm/testnet/evm',rpcPublic:'https://blockchainapi.one/api/hashburst/hvm/testnet/evm',ws:'wss://blockchainapi.one/api/hashburst/hvm/testnet/evm/ws',federation:'Testnet — separate validator set',apow:'Testnet service — non-production HBT',note:'Testnet is a separate signing domain and remains available for testing.'}
  };
  let selected='mainnet',previous={mainnet:null,testnet:null},sample=null,progress='first',busy=false,error='',rpcOK=false,sampledAt='';
  const words={unknown:'Not checked',first:'First sample; refresh to compare',advanced:'Advanced since previous sample',same:'No advancement since previous sample',decreased:'Height decreased: investigate',unavailable:'Unavailable',running:'Running',stopped:'Stopped',stale:'Detailed status unavailable',activated:'Activation height finalized',pending:'Pending',time:'Last sample',active:'ACTIVE'};
  function cfg(){return networks[selected]}
  function render(){
    const c=cfg();
    for(const n of ['mainnet','testnet']){const b=el('connect-'+n),on=selected===n;b.className='btn'+(on?' active':'');b.setAttribute('aria-pressed',on?'true':'false');}
    el('network').textContent=c.name;el('chain').textContent=c.chain+' ('+c.hex+')';el('federation').textContent=c.federation;el('apow').textContent=c.apow;
    el('rpc-url').textContent=c.rpcPublic;el('ws-url').textContent=c.ws;el('network-note').textContent=c.note;
    el('status').textContent=error?words.unavailable+': '+error:sample?(selected==='mainnet'?'Mainnet':'Testnet')+' observer responding':words.unknown;
    el('time').textContent=sampledAt?words.time+': '+sampledAt:'';
    for(const id of ['live','height','finalized','progress','peers','reactor','activation','rpc']) el(id).textContent=error?words.unavailable:words.unknown;
    if(!sample||error)return;
    el('live').textContent=sample.ok===true&&sample.reactor_running===true?words.active:words.unavailable;
    el('height').textContent=String(sample.height);el('finalized').textContent=String(sample.finalized_height);el('peers').textContent=String(sample.peer_count);el('progress').textContent=words[progress];
    el('reactor').textContent=sample.reactor_status_fresh&&sample.reactor?(sample.reactor_running?words.running:words.stopped)+' / '+sample.reactor.height+' / '+sample.reactor.round+' / '+sample.reactor.step:words.stale;
    el('activation').textContent=sample.evm_activation_height?(sample.finalized_height>=sample.evm_activation_height?words.activated:words.pending)+' ('+sample.evm_activation_height+')':words.unknown;
    el('rpc').textContent=rpcOK?(selected==='mainnet'?'Mainnet':'Testnet')+' RPC responding':words.unavailable;
  }
  async function refresh(){
    if(busy)return;busy=true;el('refresh').disabled=true;const c=cfg(),requested=selected;
    try{
      const r=await fetch(c.network,{cache:'no-store',signal:AbortSignal.timeout(15000)});if(!r.ok)throw Error('HTTP '+r.status);const d=await r.json();
      if(d.ok!==true||d.chain_id!==c.chain||d.network!==requested||d.role!=='observer'||d.node_id!==c.node||!Number.isSafeInteger(d.height)||!Number.isSafeInteger(d.finalized_height)||!Number.isSafeInteger(d.peer_count)||d.peer_count<0)throw Error('network/identity');
      progress=previous[requested]===null?'first':d.finalized_height>previous[requested]?'advanced':d.finalized_height===previous[requested]?'same':'decreased';
      previous[requested]=d.finalized_height;if(selected!==requested)return;sample=d;error='';sampledAt=new Date().toISOString();rpcOK=false;
      try{const rr=await fetch(c.rpc,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({jsonrpc:'2.0',id:1,method:'eth_chainId',params:[]}),signal:AbortSignal.timeout(8000)});const v=rr.ok?await rr.json():null;rpcOK=!!v&&!v.error&&v.result===c.hex;}catch(_){}
    }catch(e){if(selected===requested){error=e.message;sample=null;previous[requested]=null;}}
    finally{busy=false;el('refresh').disabled=false;render();}
  }
  function select(n){if(!networks[n]||busy)return;selected=n;sample=null;error='';sampledAt='';rpcOK=false;render();refresh();}
  document.getElementById('nav-hvm').addEventListener('click',refresh);el('refresh').addEventListener('click',refresh);
  el('connect-mainnet').addEventListener('click',()=>select('mainnet'));el('connect-testnet').addEventListener('click',()=>select('testnet'));
  setInterval(()=>{if(root.classList.contains('active')&&!document.hidden)refresh();},30000);render();
})();
