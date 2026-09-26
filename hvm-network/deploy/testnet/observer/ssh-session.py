"""One dedicated SSH process per node; no OpenSSH multiplexing sockets."""
import json,os,selectors,shlex,subprocess,time
PREFIX=b'HB_ACTION_JSON:'
RUNNER=r'''
import contextlib,json,sys,traceback
source=json.loads(sys.stdin.readline())['source']
namespace={'__name__':'hvm_remote_actions'}
with contextlib.redirect_stdout(sys.stderr):exec(compile(source,'node-action.py','exec'),namespace)
print('HB_ACTION_JSON:'+json.dumps({'ready':True}),flush=True)
for line in sys.stdin:
    request=json.loads(line)
    try:
        with contextlib.redirect_stdout(sys.stderr):result=namespace['main'](request['payload'])
        response={'id':request['id'],'ok':True,'result':result}
    except Exception as exc:
        # Errors are returned once in the structured reply.
        response={'id':request['id'],'ok':False,'error':str(exc)}
    print('HB_ACTION_JSON:'+json.dumps(response),flush=True)
'''
def ssh_command(ip):
    return ['ssh','-T','-o','ControlMaster=no','-o','ControlPath=none','-o','ControlPersist=no','-o','BatchMode=no','-o','StrictHostKeyChecking=ask','-o','ConnectTimeout=10','-o','ServerAliveInterval=5','-o','ServerAliveCountMax=3','root@'+ip,'python3 -u -c '+shlex.quote(RUNNER)]
class Session:
    def __init__(self,command,source):
        self.process=subprocess.Popen(command,stdin=subprocess.PIPE,stdout=subprocess.PIPE,bufsize=0)
        self.buffer=b'';self.sequence=0;self.broken=False
        self.selector=selectors.DefaultSelector();self.selector.register(self.process.stdout,selectors.EVENT_READ)
        try:
            self.send({'source':source})
            if self.receive(180)!={'ready':True}:raise RuntimeError('remote handshake mismatch')
        except BaseException:self.close();raise
    def send(self,data):
        raw=(json.dumps(data)+'\n').encode();position=0
        while position<len(raw):
            n=os.write(self.process.stdin.fileno(),raw[position:])
            if n<=0:raise RuntimeError('SSH input closed')
            position+=n
    def receive(self,timeout):
        deadline=time.monotonic()+timeout
        while True:
            while b'\n' in self.buffer:
                line,self.buffer=self.buffer.split(b'\n',1)
                if line.startswith(PREFIX):return json.loads(line[len(PREFIX):])
            remaining=deadline-time.monotonic()
            if remaining<=0:raise TimeoutError('SSH action timed out')
            if not self.selector.select(remaining):raise TimeoutError('SSH action timed out')
            chunk=os.read(self.process.stdout.fileno(),65536)
            if not chunk:raise RuntimeError('dedicated SSH connection closed')
            self.buffer+=chunk
            if len(self.buffer)>4*1024*1024:raise RuntimeError('oversized SSH output')
    def call(self,payload):
        if self.broken:raise RuntimeError('SSH connection unavailable; no action retried')
        self.sequence+=1
        try:
            self.send({'id':self.sequence,'payload':payload});reply=self.receive(payload.get('_timeout',30))
            if reply.get('id')!=self.sequence:raise RuntimeError('SSH action response mismatch')
        except BaseException:self.broken=True;self.close();raise
        if reply.get('ok') is not True:raise RuntimeError(reply.get('error','remote action failed'))
        return reply['result']
    def close(self):
        # Closing this transport never stops or rolls back an HVM service.
        if self.process.stdin and not self.process.stdin.closed:self.process.stdin.close()
        try:self.process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            self.process.terminate()
            try:self.process.wait(timeout=3)
            except subprocess.TimeoutExpired:self.process.kill();self.process.wait()
        self.selector.close()
        if self.process.stdout:self.process.stdout.close()
