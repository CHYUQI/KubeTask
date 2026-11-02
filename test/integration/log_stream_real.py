#!/usr/bin/env python3
"""Regression test using a real cluster, real Pods and ServiceAccount tokens.

Requires KUBECONFIG and an unused local API port. No envtest or injected status.
Example: python3 test/integration/log_stream_real.py --manager ./manager
"""
import argparse
import json
import os
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manager', required=True)
    parser.add_argument('--kubectl', default='kubectl')
    parser.add_argument('--port', type=int, default=18080)
    args = parser.parse_args()
    prefix = 'sse-eof-' + uuid.uuid4().hex[:8]
    names = [prefix + '-done', prefix + '-live']
    base = f'http://127.0.0.1:{args.port}'

    def kubectl(*words, obj=None):
        return subprocess.check_output(
            [args.kubectl, *words], text=True,
            input=json.dumps(obj) if obj is not None else None,
        )

    def wait(predicate, seconds=120):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.5)
        raise TimeoutError('real cluster condition did not become ready')

    env = os.environ.copy()
    env.update(KUBETASK_AUTH_ENABLED='true', KUBETASK_AUTH_AUDIENCE='sse-regression',
               KUBETASK_API_PORT=str(args.port), KUBETASK_LEADER_ELECT='false',
               KUBETASK_METRICS_BIND_ADDRESS='0', KUBETASK_WEBHOOK_ENABLED='false')
    with tempfile.TemporaryFile() as log:
        manager = subprocess.Popen([os.path.abspath(args.manager)], env=env, stdout=log, stderr=log)
        try:
            def ready():
                if manager.poll() is not None:
                    raise RuntimeError('manager exited before readiness')
                try:
                    with urllib.request.urlopen(base + '/healthz', timeout=1) as response:
                        return response.status == 200
                except (OSError, urllib.error.URLError):
                    return False
            wait(ready, 30)
            kubectl('create', 'serviceaccount', prefix, '-n', 'default')
            token = kubectl('create', 'token', prefix, '-n', 'default',
                            '--audience=sse-regression').strip()
            for name, command in zip(names, ['echo start; echo end', 'echo start; sleep 8; echo end']):
                kubectl('create', '-f', '-', obj={
                    'apiVersion': 'kubetask.kubetask.io/v1', 'kind': 'Task',
                    'metadata': {'name': name},
                    'spec': {'type': 'OneTime', 'image': 'busybox:1.37.0',
                             'command': ['sh', '-c', command], 'backoffLimit': 0},
                })
                if name == names[0]:
                    wait(lambda: json.loads(kubectl('get', 'task', name, '-o', 'json')).get('status', {}).get('phase') == 'Succeeded')
                    modes = [False, True]
                else:
                    wait(lambda: any(p['metadata']['name'].startswith(name + '-') and
                                     p.get('status', {}).get('phase') == 'Running'
                                     for p in json.loads(kubectl('get', 'pods', '-n', 'default', '-o', 'json'))['items']))
                    modes = [True]
                for follow in modes:
                    request = urllib.request.Request(
                        base + f'/api/v1/tasks/{name}/logs?follow={str(follow).lower()}',
                        headers={'Authorization': 'Bearer ' + token})
                    began = time.monotonic()
                    with urllib.request.urlopen(request, timeout=15 if name == names[1] else 3) as response:
                        body = response.read()  # Must receive EOF without client cancellation.
                        assert response.status == 200
                        assert response.headers.get_content_type() == 'text/event-stream'
                    assert b'data: start\n\n' in body and b'data: end\n\n' in body
                    elapsed = time.monotonic() - began
                    if name == names[1]:
                        assert elapsed > 3, 'live stream ended inside the authentication timeout window'
                    print(json.dumps({'task': name, 'follow': follow, 'eof': True,
                                      'seconds': round(elapsed, 3)}), flush=True)
        finally:
            try:
                for name in names:
                    kubectl('delete', 'task', name, '--ignore-not-found', '--wait=false')
                kubectl('delete', 'serviceaccount', prefix, '-n', 'default', '--ignore-not-found')
                wait(lambda: all(not kubectl('get', 'task', name, '--ignore-not-found', '-o', 'name').strip()
                                 for name in names), 20)
            finally:
                manager.terminate()
                manager.wait(timeout=20)
                if manager.returncode:
                    log.seek(0)
                    print(log.read().decode(errors='replace'))


if __name__ == '__main__':
    main()
