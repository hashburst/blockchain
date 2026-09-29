const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const nodes=new Map();
function element(id){if(!nodes.has(id))nodes.set(id,{textContent:'',disabled:false,handlers:{},classList:{contains:()=>true},setAttribute(){},addEventListener(n,f){this.handlers[n]=f;},querySelectorAll(){return []}});return nodes.get(id)}
let height=10,bad=false;
const context={document:{hidden:false,getElementById:element},localStorage:{getItem:()=>null,setItem(){}},setInterval(){},AbortSignal:{timeout(){}},Date,fetch:async(url)=>({ok:true,json:async()=>url.endsWith('/evm')?{result:'0x484202'}:{ok:true,chain_id:bad?4735489:4735490,node_id:'hvm-testnet-ingress',role:'observer',finalized_height:height,peer_count:4,reactor_status_fresh:false}})};
vm.runInNewContext(fs.readFileSync(__dirname+'/dashboard/card.js','utf8'),context);
(async()=>{
 await element('hvm-refresh').handlers.click();assert.equal(element('hvm-finalized').textContent,'10');
 height++;await element('hvm-refresh').handlers.click();assert.equal(element('hvm-progress').textContent,'Avanzata dal campione precedente');
 element('hvm-en').handlers.click();assert.equal(element('hvm-progress').textContent,'Advanced since previous sample');
 bad=true;await element('hvm-refresh').handlers.click();assert.equal(element('hvm-finalized').textContent,'Unavailable');
 assert.equal(element('hvm-refresh').disabled,false);console.log('DASHBOARD_REFRESH_LANGUAGE_STALE_GUARDS_OK');
})().catch(e=>{console.error(e);process.exitCode=1});
