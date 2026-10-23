"""Check local-only launcher configuration without starting any services."""
import contextlib
import io
import json
import os
from pathlib import Path
import runpy
import tempfile
import unittest
from unittest.mock import patch


LAUNCHER = Path(__file__).with_name('start-native-dev.py').resolve()
MAIN = runpy.run_path(str(LAUNCHER))['main']


class LauncherTest(unittest.TestCase):
    def invoke(self, config):
        # The launcher checks existence, but the mocked kubectl reads no file.
        with tempfile.TemporaryDirectory() as directory:
            kubeconfig = Path(directory) / 'config'
            kubeconfig.touch()
            args = [str(LAUNCHER), '--kubeconfig', str(kubeconfig),
                    '--access-url', 'http://172.20.0.3:3100']
            with patch('sys.argv', args), \
                    patch('subprocess.check_output', return_value=json.dumps(config)), \
                    patch('os.chdir'), patch('os.execve') as execute, \
                    contextlib.redirect_stderr(io.StringIO()) as stderr:
                try:
                    MAIN()
                except SystemExit as error:
                    execute.assert_not_called()
                    return error.code, stderr.getvalue(), None
                return 0, stderr.getvalue(), execute.call_args

    def config(self):
        return {'clusters': [{'cluster': {'server': 'https://127.0.0.1:22643'}}],
                'users': [{'user': {}}]}

    def test_local_launch_sanitizes_credentials(self):
        with patch.dict(os.environ, {'CODER_SESSION_TOKEN': 'fixture',
                                     'AWS_SESSION_TOKEN': 'fixture',
                                     'TF_TOKEN_example': 'fixture',
                                     'https_proxy': 'fixture'}):
            status, _, call = self.invoke(self.config())
        self.assertEqual(status, 0)
        executable, args, env = call.args
        self.assertEqual(executable, './scripts/develop.sh')
        self.assertEqual(args[args.index('--experiments') + 1], 'template-resources')
        self.assertNotIn('--network-policy-module-path', args)
        self.assertEqual(args[args.index('--port') + 1], '3100')
        self.assertEqual(args[args.index('--web-port') + 1], '8180')
        self.assertEqual(env['KUBE_CONFIG_PATH'], env['KUBECONFIG'])
        self.assertEqual(env['AWS_SHARED_CREDENTIALS_FILE'], os.devnull)
        for key in ('CODER_SESSION_TOKEN', 'AWS_SESSION_TOKEN',
                    'TF_TOKEN_example', 'https_proxy'):
            self.assertNotIn(key, env)

    def test_rejects_kubeconfig_proxy(self):
        for proxy in ('http://192.0.2.10:8080', 'http://127.0.0.1:8080'):
            with self.subTest(proxy=proxy):
                config = self.config()
                config['clusters'][0]['cluster']['proxy-url'] = proxy
                status, error, _ = self.invoke(config)
                self.assertEqual(status, 2)
                self.assertIn('Kubeconfig proxies are not supported', error)

    def test_rejects_remote_api(self):
        config = self.config()
        config['clusters'][0]['cluster']['server'] = 'https://192.0.2.1:6443'
        status, error, _ = self.invoke(config)
        self.assertEqual(status, 2)
        self.assertIn('HTTPS loopback Kubernetes API', error)

    def test_rejects_credential_plugins(self):
        for key in ('exec', 'auth-provider'):
            with self.subTest(plugin=key):
                config = self.config()
                config['users'][0]['user'][key] = {'name': 'fixture'}
                status, error, _ = self.invoke(config)
                self.assertEqual(status, 2)
                self.assertIn('Credential plugins are not supported', error)


if __name__ == '__main__':
    unittest.main()
