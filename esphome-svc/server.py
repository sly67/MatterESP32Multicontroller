#!/usr/bin/env python3
import json, os, re, signal, subprocess, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 6052
_lock = threading.Lock()
_proc = None

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args): pass

    def do_GET(self):
        if self.path == '/health':
            self._send(200, b'ok')
        else:
            self._send(404, b'not found')

    def do_DELETE(self):
        global _proc
        if not self.path.startswith('/compile'):
            self._send(404, b'not found'); return
        with _lock:
            p = _proc
        if p and p.poll() is None:
            try:
                p.send_signal(signal.SIGTERM)
            except ProcessLookupError:
                pass
        self._send(204, b'')

    def do_POST(self):
        global _proc
        if not self.path.startswith('/compile/'):
            self._send(404, b'not found'); return
        device = self.path[len('/compile/'):]
        if not device:
            self._send(400, b'missing device'); return

        # Fix 4: validate device name to prevent path traversal
        if not re.fullmatch(r'[a-zA-Z0-9_-]+', device):
            self._send(400, b'invalid device name'); return

        cfg_path = f'/config/{device}/config.yaml'
        if not os.path.exists(cfg_path):
            body = json.dumps({'result': 'error', 'message': 'config not found'}).encode()
            self._send(400, body); return

        # Fix 3: hold lock across 409 check AND Popen to prevent TOCTOU race
        # Fix 6: send 200 headers only after Popen succeeds
        with _lock:
            if _proc is not None and _proc.poll() is None:
                self._send(409, b'compile in progress'); return
            proc = subprocess.Popen(
                ['esphome', 'compile', cfg_path],
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
            )
            _proc = proc

        # Fix 1 (ThreadingHTTPServer) + Fix 2 (no Transfer-Encoding: chunked)
        self.send_response(200)
        self.send_header('Content-Type', 'application/x-ndjson')
        self.end_headers()

        try:
            for line in proc.stdout:
                line = line.rstrip('\n')
                if line:
                    msg = json.dumps({'log': line}) + '\n'
                    self.wfile.write(msg.encode())
                    self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            proc.send_signal(signal.SIGTERM)
        finally:
            proc.wait()
            with _lock:
                _proc = None

        code = proc.returncode
        if code == 0:
            result = json.dumps({'result': 'ok'}) + '\n'
        else:
            result = json.dumps({'result': 'error', 'code': code}) + '\n'
        try:
            self.wfile.write(result.encode())
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _send(self, code, body=b''):
        self.send_response(code)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

if __name__ == '__main__':
    server = ThreadingHTTPServer(('0.0.0.0', PORT), Handler)
    print(f'ESPHome sidecar listening on :{PORT}', flush=True)
    server.serve_forever()
