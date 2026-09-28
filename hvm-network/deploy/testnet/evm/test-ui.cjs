const vm=require('node:vm'),fs=require('node:fs'),assert=require('node:assert/strict');
const source=fs.readFileSync(__dirname+'/metamask-canary.js','utf8');
const elements=new Map(),listeners=new Map();
function element(){return {textContent:'',value:'',disabled:false,children:[],replaceChildren(){this.children=[];this.value=''},append(o){this.children.push(o);if(this.value==='')this.value=o.value}}}
const document={documentElement:{},getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id)},createElement:element};
const wallet={request:async()=>{}};
const window={ethereum:{request:async()=>{},isMetaMask:false},addEventListener(type,fn){listeners.set(type,fn)},dispatchEvent(e){if(e.type==='eip6963:requestProvider')listeners.get('eip6963:announceProvider')({detail:{info:{name:'MetaMask',rdns:'io.metamask'},provider:wallet}})}};
const context=vm.createContext({document,window,Event:class{constructor(type){this.type=type}},localStorage:{getItem(){return null},setItem(){}},setTimeout,clearTimeout,URL,console});
vm.runInContext(source,context);
assert.equal(vm.runInContext('walletEntries.length',context),1);
assert.equal(vm.runInContext('walletEntries[0].provider',context),wallet);
vm.runInContext('requestWallets();requestWallets()',context);
assert.equal(vm.runInContext('walletEntries.length',context),1);
vm.runInContext("setLanguage('en')",context);
assert.equal(document.documentElement.lang,'en');assert.equal(elements.get('connect').textContent,'Connect MetaMask to testnet');
vm.runInContext("setLanguage('it')",context);assert.equal(elements.get('connect').textContent,'Collega MetaMask alla testnet');
(async()=>{await vm.runInContext('discoverWallets()',context);assert.equal(vm.runInContext('walletEntries.length',context),2);assert.equal(elements.get('wallet-select').value,'0');console.log('PASS EIP6963 discovery despite unrelated window.ethereum; deduplication; IT/EN; legacy fallback; selection preservation')})();
