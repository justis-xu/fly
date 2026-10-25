#!/usr/bin/env python3
"""Real fly binary, fresh HOME/FLY_HOME, private ports and controlled local endpoints.
No real subscription, system proxy, browser profile or external website is used.
"""
import argparse
import contextlib
import http.client
import http.server
import json
import os
from pathlib import Path
import select
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.parse

ROOT = Path(__file__).resolve().parents[2]


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def log_message(self, *args):
        pass

    def reply(self, body, status=200, content_type='text/plain'):
        if isinstance(body, str):
            body = body.encode()
        self.send_response(status)
        self.send_header('Content-Length', str(len(body)))
        self.send_header('Content-Type', content_type)
        self.send_header('Connection', 'close')
        self.end_headers()
        if self.command != 'HEAD':
            self.wfile.write(body)
        self.close_connection = True


@contextlib.contextmanager
def serve(handler):
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), handler)
    server.daemon_threads = True
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield server
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', default=str(ROOT / 'fly'))
    parser.add_argument('--install', action='store_true', help='also exercise make install into a private directory')
    args = parser.parse_args()
    binary = Path(args.binary).resolve()
    require(binary.is_file(), 'build fly first')
    with tempfile.TemporaryDirectory(prefix='fly-clean-room-') as temporary:
        workspace = Path(temporary)
        if args.install:
            subprocess.run(['make', 'install', 'DEST=' + str(workspace / 'bin')], cwd=ROOT, check=True)
            binary = workspace / 'bin' / 'fly'
        home, data = workspace / 'home', workspace / 'data'
        home.mkdir()
        env = {'HOME': str(home), 'FLY_HOME': str(data), 'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'LANG': 'C', 'LC_ALL': 'C'}
        proxy_port, admin_port = free_port(), free_port()
        while admin_port == proxy_port:
            admin_port = free_port()

        def cli(*arguments, success=True):
            result = subprocess.run([str(binary), *arguments], env=env, capture_output=True, text=True, timeout=130)
            if success:
                require(result.returncode == 0, 'CLI failed: ' + result.stdout + result.stderr)
            else:
                require(result.returncode != 0, 'invalid CLI operation succeeded')
            return result.stdout + result.stderr

        def api(path, method='GET', payload=None):
            connection = http.client.HTTPConnection('127.0.0.1', admin_port, timeout=100)
            body = None if payload is None else json.dumps(payload)
            connection.request(method, '/api/' + path, body, {'Content-Type': 'application/json'})
            response = connection.getresponse()
            content = response.read()
            code = response.status
            connection.close()
            try:
                result = json.loads(content)
            except ValueError:
                result = content.decode()
            return code, result

        def verify_listeners():
            if not shutil.which('lsof'):
                print('NOTE: process listener inventory skipped because lsof is unavailable')
                return
            record = json.loads((data / 'fly.pid').read_text())
            output = subprocess.check_output(['lsof', '-nP', '-a', '-p', str(record['pid']), '-iTCP', '-sTCP:LISTEN', '-Fn'], text=True)
            listeners = {line[1:] for line in output.splitlines() if line.startswith('n')}
            require(listeners == {'127.0.0.1:' + str(admin_port), '127.0.0.1:' + str(proxy_port)}, 'unexpected listener: ' + str(listeners))

        def wait_ready(expected=True):
            deadline = time.monotonic() + 35
            latest = None
            while time.monotonic() < deadline:
                try:
                    code, latest = api('info')
                    if code == 200 and latest['ready'] == expected and not latest['starting']:
                        return latest
                except (OSError, http.client.HTTPException):
                    pass
                time.sleep(0.1)
            raise AssertionError('server readiness failed: ' + str(latest))

        class Target(Handler):
            def do_GET(self):
                self.reply('controlled-target')

        counters = {'A': 0, 'B': 0}
        bodies = {}
        allowed_destination = ''

        def relay(name):
            class Relay(Handler):
                def do_GET(self):
                    # Explicit backup proxy for an otherwise unreachable subscription URL.
                    if name == 'B' and self.path.startswith('http://127.0.0.1:1/sub-b'):
                        self.reply(bodies['B'])
                    else:
                        self.reply('controlled destination only', 502)

                def do_CONNECT(self):
                    if self.path != allowed_destination:
                        self.reply('controlled destination only', 502)
                        return
                    host, port = self.path.rsplit(':', 1)
                    remote = socket.create_connection((host, int(port)), timeout=3)
                    counters[name] += 1
                    self.send_response(200)
                    self.end_headers()
                    self.close_connection = True
                    try:
                        while True:
                            ready, _, _ = select.select([self.connection, remote], [], [], 3)
                            if not ready:
                                return
                            for source in ready:
                                content = source.recv(65536)
                                if not content:
                                    return
                                (remote if source is self.connection else self.connection).sendall(content)
                    except OSError:
                        return
                    finally:
                        remote.close()
            return Relay

        offline = False

        class Source(Handler):
            def do_GET(self):
                path = urllib.parse.urlsplit(self.path).path
                if offline:
                    self.reply('subscription service offline', 503)
                elif path == '/a':
                    self.reply(bodies['A'])
                elif path == '/invalid':
                    self.reply('<!doctype html><title>Account login</title>')
                elif path == '/broken-rules':
                    self.reply(bodies['broken'])
                elif path in ('/rules-a', '/rules-b'):
                    if self.headers.get('X-Provider-Key') != 'demo':
                        self.reply('auth required', 401)
                    else:
                        self.reply('payload:\n- DOMAIN,' + ('a' if path.endswith('a') else 'b') + '.example.org\n')
                else:
                    self.reply('not found', 404)

        started = False
        try:
            with serve(Target) as target, serve(relay('A')) as relay_a, serve(relay('B')) as relay_b, serve(Source) as source:
                allowed_destination = '127.0.0.1:' + str(target.server_port)
                target_url = 'http://' + allowed_destination + '/probe'
                source_url = 'http://127.0.0.1:' + str(source.server_port)
                backup_url = 'http://127.0.0.1:' + str(relay_b.server_port)

                def profile(name, port, suffix):
                    return f'''mixed-port: 9000
external-controller: 127.0.0.1:9090
allow-lan: true
tun: {{enable: true}}
proxies:
- {{name: node-{name}, type: http, server: 127.0.0.1, port: {port}}}
rule-providers:
  sample:
    type: http
    behavior: classical
    url: {source_url}/rules-{suffix}
    header: {{X-Provider-Key: [demo]}}
    interval: 3600
rules:
- RULE-SET,sample,DIRECT
- MATCH,ProviderGroup
'''
                bodies['A'] = profile('A', relay_a.server_port, 'a')
                bodies['B'] = profile('B', relay_b.server_port, 'b')
                bodies['broken'] = bodies['A'].replace('MATCH,ProviderGroup', 'INVALID-RULE,example.org,ProviderGroup')
                cli('init', '--port', str(proxy_port), '--admin', '127.0.0.1:' + str(admin_port))
                cli('start')
                started = True
                info = wait_ready(False)
                require(info['needs_subscription'] and info['nodes'] == 0, 'fresh HOME borrowed personal configuration')
                require(not (home / '.fly').exists(), 'FLY_HOME was ignored')
                require('已在运行' in cli('start'), 'duplicate start did not reuse owned daemon')
                print('PASS: fresh HOME, FLY_HOME, no environment proxy, first-run GUI and duplicate start')

                code, _ = api('subscription', 'POST', {'url': source_url + '/a?token=demo-secret'})
                require(code == 200, 'valid source A rejected')
                info = wait_ready()
                require(info['nodes'] == 1 and info['proxy_port'] == proxy_port, 'subscription overrode listener configuration')
                verify_listeners()
                code, page = api('rule-providers/sample?q=a.example&offset=0&limit=100')
                require(code == 200 and page['items'] == ['DOMAIN,a.example.org'], 'authenticated rule resource is not readable')
                print('PASS: source A, ignored extra listeners/TUN, authenticated rule download and provider search')

                def http_proxy():
                    connection = http.client.HTTPConnection('127.0.0.1', proxy_port, timeout=5)
                    connection.request('GET', target_url, headers={'Connection': 'close'})
                    response = connection.getresponse()
                    body = response.read()
                    status = response.status
                    connection.close()
                    return status, body

                def socks_proxy():
                    def exact(sock, size):
                        out = b''
                        while len(out) < size:
                            chunk = sock.recv(size - len(out))
                            if not chunk:
                                raise AssertionError('SOCKS reply truncated')
                            out += chunk
                        return out
                    with socket.create_connection(('127.0.0.1', proxy_port), timeout=5) as sock:
                        sock.sendall(b'\x05\x01\x00')
                        require(exact(sock, 2) == b'\x05\x00', 'SOCKS5 greeting failed')
                        sock.sendall(b'\x05\x01\x00\x01' + socket.inet_aton('127.0.0.1') + target.server_port.to_bytes(2, 'big'))
                        reply = exact(sock, 4)
                        require(reply[:2] == b'\x05\x00', 'SOCKS connect failed')
                        if reply[3] == 1:
                            exact(sock, 6)
                        elif reply[3] == 4:
                            exact(sock, 18)
                        else:
                            exact(sock, exact(sock, 1)[0] + 2)
                        sock.sendall(('GET /probe HTTP/1.1\r\nHost: ' + allowed_destination + '\r\nConnection: close\r\n\r\n').encode())
                        content = b''
                        while True:
                            part = sock.recv(65536)
                            if not part:
                                break
                            content += part
                        require(b'200 OK' in content and b'controlled-target' in content, 'SOCKS forwarding failed')

                require(http_proxy() == (200, b'controlled-target'), 'HTTP proxy did not reach controlled target')
                socks_proxy()
                require(counters['A'] >= 2, 'requests silently bypassed the selected subscription node')
                print('PASS: HTTP and SOCKS5 forwarding through the real configured upstream')

                old_url = api('subscription')[1]['url']
                for path in ('/invalid', '/broken-rules'):
                    code, error = api('subscription', 'POST', {'url': source_url + path + '?token=never-print-this'})
                    require(code >= 400, 'invalid replacement accepted')
                    require('never-print-this' not in str(error), 'private URL leaked in error')
                    require(api('subscription')[1]['url'] == old_url, 'failed replacement lost source A')
                    require(http_proxy()[0] == 200, 'failed replacement broke active forwarding')
                output = cli('init', '--url', source_url + '/invalid?token=never-print-this', success=False)
                require('never-print-this' not in output and api('subscription')[1]['url'] == old_url, 'CLI bypassed transactional validation')
                print('PASS: bad HTML, invalid core rules and invalid CLI replacement preserve working source')

                code, _ = api('subscription', 'POST', {'url': 'http://127.0.0.1:1/sub-b', 'download_proxy': backup_url})
                require(code == 200, 'backup-download source B rejected')
                info = wait_ready()
                require(info['download_route'] == '备用代理', 'backup download route not reported')
                require('node-B' in api('proxies')[1]['proxies'] and 'node-A' not in api('proxies')[1]['proxies'], 'source switch reused old nodes')
                require(http_proxy()[0] == 200 and counters['B'] > 0, 'source B upstream not used')
                code, _ = api('routing', 'PUT', {'user_rules': [{'match': 'ip_cidr', 'value': '127.0.0.1/32', 'action': 'reject'}]})
                require(code == 200 and http_proxy()[0] != 200, 'reject override did not reject')
                code, _ = api('routing', 'PUT', {'user_rules': [{'match': 'domain', 'value': 'bad,value', 'action': 'direct'}]})
                require(code >= 400 and http_proxy()[0] != 200, 'invalid rule edit destroyed the saved reject rule')
                code, _ = api('routing', 'PUT', {'user_rules': [{'match': 'ip_cidr', 'value': '127.0.0.1/32', 'action': 'direct'}]})
                before = counters['B']
                require(code == 200 and http_proxy()[0] == 200 and counters['B'] == before, 'direct override did not bypass upstream')
                print('PASS: fallback download, new source identity, editable reject/direct rules and invalid-edit rollback')

                offline = True
                cli('stop')
                started = False
                cli('start')
                started = True
                info = wait_ready()
                require(info['last_err'] and 'node-B' in api('proxies')[1]['proxies'], 'offline startup used an unrelated source cache')
                require(api('routing')[1]['user_rules'][0]['action'] == 'direct', 'restart lost user overrides')
                require(api('rule-providers/sample')[0] == 200, 'offline restart lost rule resource cache')
                require(http_proxy()[0] == 200, 'subscription service outage broke cached forwarding')
                print('PASS: offline subscription restart with matching node/rule caches and persistent overrides')

                content = 'proxies:\n- {name: imported-node, type: http, server: 127.0.0.1, port: ' + str(relay_b.server_port) + '}\nrules:\n- MATCH,DIRECT\n'
                code, _ = api('subscription', 'POST', {'content': content})
                require(code == 200, 'local import failed')
                info = wait_ready()
                require(info['imported'] and not info['needs_subscription'] and info['nodes'] == 1, 'import is presented as missing subscription')
                cli('restart')
                require(wait_ready()['imported'] and http_proxy()[0] == 200, 'local import cannot restart offline')
                for name in ('fly.yaml', 'rules.yaml'):
                    require((data / name).stat().st_mode & 0o777 == 0o600, 'private state permissions incorrect')
                cached = list((data / 'subscriptions').glob('*.json'))
                require(len(cached) >= 3 and all(path.stat().st_mode & 0o777 == 0o600 for path in cached), 'source caches are not isolated/private')
                verify_listeners()
                print('PASS: local file offline restart, private source caches, expected loopback listeners')
                cli('stop')
                started = False
                require(not (data / 'fly.pid').exists(), 'stop left an active PID record')
                print('PASS: CLI start/stop/restart and cleanup of the owned instance')
        finally:
            if started:
                cli('stop')
    print('CLEAN-ROOM ACCEPTANCE PASSED')


if __name__ == '__main__':
    main()
