// HVM_NETWORK_CARD_V5_EN
(() => {
  'use strict';
  const root=document.getElementById('view-hvm'), el=id=>document.getElementById('hvm-'+id);
  let previous=null,sample=null,progress='first',busy=false,error='',rpcOK=false,sampledAt='';
  const words={unknown:'Not checked',first:'First sample; refresh to compare',advanced:'Advanced since previous sample',same:'No advancement since previous sample',decreased:'Height decreased: investigate',responding:'Mainnet observer responding',unavailable:'Unavailable',running:'Running',stopped:'Stopped',stale:'Detailed status unavailable',activated:'Activation height finalized',pending:'Pending',rpc:'Mainnet RPC responding',time:'Last sample',active:'ACTIVE'};
  function render(){
    el('status').textContent=error?words.unavailable+': '+error:sample?words.responding:words.unknown;
    el('time').textContent=sampledAt?words.time+': '+sampledAt:'';
    for(const id of ['live','height','finalized','progress','peers','reactor','activation','rpc']) el(id).textContent=error?words.unavailable:words.unknown;
    if(!sample||error)return;
    el('live').textContent=sample.ok===true&&sample.reactor_running===true?words.active:words.unavailable;
    el('height').textContent=String(sample.height);
    el('finalized').textContent=String(sample.finalized_height);
    el('peers').textContent=String(sample.peer_count);
    el('progress').textContent=words[progress];
    el('reactor').textContent=sample.reactor_status_fresh&&sample.reactor?(sample.reactor_running?words.running:words.stopped)+' / '+sample.reactor.height+' / '+sample.reactor.round+' / '+sample.reactor.step:words.stale;
    el('activation').textContent=sample.evm_activation_height?(sample.finalized_height>=sample.evm_activation_height?words.activated:words.pending)+' ('+sample.evm_activation_height+')':words.unknown;
    el('rpc').textContent=rpcOK?words.rpc:words.unavailable;
  }
  async function refresh(){
    if(busy)return; busy=true;el('refresh').disabled=true;
    try{
      const r=await fetch('/api/hashburst/hvm/mainnet/network',{cache:'no-store',signal:AbortSignal.timeout(15000)});
      if(!r.ok)throw Error('HTTP '+r.status);
      const d=await r.json();
      if(d.ok!==true||d.chain_id!==4735489||d.network!=='mainnet'||d.role!=='observer'||d.node_id!=='hvm-mainnet-observer-01'||!Number.isSafeInteger(d.height)||!Number.isSafeInteger(d.finalized_height)||!Number.isSafeInteger(d.peer_count)||d.peer_count<0)throw Error('network/identity');
      progress=previous===null?'first':d.finalized_height>previous?'advanced':d.finalized_height===previous?'same':'decreased';
      previous=d.finalized_height;sample=d;error='';sampledAt=new Date().toISOString();rpcOK=false;
      try{
        const rr=await fetch('/api/hashburst/hvm/mainnet/evm',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({jsonrpc:'2.0',id:1,method:'eth_chainId',params:[]}),signal:AbortSignal.timeout(8000)});
        const v=rr.ok?await rr.json():null;rpcOK=!!v&&!v.error&&v.result==='0x484201';
      }catch(_){}
    }catch(e){error=e.message;sample=null;previous=null;}
    finally{busy=false;el('refresh').disabled=false;render();}
  }
  document.getElementById('nav-hvm').addEventListener('click',refresh);el('refresh').addEventListener('click',refresh);
  setInterval(()=>{if(root.classList.contains('active')&&!document.hidden)refresh();},30000);render();
})();
