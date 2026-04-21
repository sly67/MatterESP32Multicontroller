#!/usr/bin/env python3
import json, os, signal, subprocess, threading
from http.server import BaseHTTPRequestHandler, HTTPServer

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
            p.send_signal(signal.SIGTERM)
        self._send(204, b'')

    def do_POST(self):
        global _proc
        if not self.path.startswith('/compile/'):
            self._send(404, b'not found'); return
        device = self.path[len('/compile/'):]
        if not device:
            self._send(400, b'missing device'); return

        with _lock:
            if _proc is not None and _proc.poll() is None:
                self._send(409, b'compile in progress'); return

        cfg_path = f'/config/{device}/config.yaml'
        if not os.path.exists(cfg_path):
            body = json.dumps({'result': 'error', 'message': 'config not found'}).encode()
            self._send(400, body); return

        self.send_response(200)
        self.send_header('Content-Type', 'application/x-ndjson')
        self.send_header('Transfer-Encoding', 'chunked')
        self.end_headers()

        proc = subprocess.Popen(
            ['esphome', 'compile', cfg_path],
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        )
        with _lock:
            _proc = proc

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
    server = HTTPServer(('0.0.0.0', PORT), Handler)
    print(f'ESPHome sidecar listening on :{PORT}', flush=True)
    server.serve_forever()
