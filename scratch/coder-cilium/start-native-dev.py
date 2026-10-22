"""Start the native network-policy demo against an existing local cluster."""
import argparse
import ipaddress
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import urlparse


def main():
    root = Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--kubeconfig', required=True, type=Path)
    parser.add_argument('--access-url', required=True,
                        help='Coder API URL reachable from workspace pods, including port 3100')
    parser.add_argument('--terraform-cli-config', type=Path,
                        help='Optional explicit provider mirror configuration')
    args = parser.parse_args()
    kubeconfig = args.kubeconfig.expanduser().resolve(strict=True)
    config = json.loads(subprocess.check_output([
        'kubectl', '--kubeconfig', str(kubeconfig), '--context', 'default',
        'config', 'view', '--minify', '-o', 'json',
    ], text=True))
    cluster = config['clusters'][0]['cluster']
    if cluster.get('proxy-url'):
        parser.error('Kubeconfig proxies are not supported by this local-only launcher')
    server = urlparse(cluster['server'])
    if server.scheme != 'https' or not ipaddress.ip_address(server.hostname).is_loopback:
        parser.error('The default context must use an HTTPS loopback Kubernetes API')
    if any('exec' in user.get('user', {}) or 'auth-provider' in user.get('user', {})
           for user in config.get('users', [])):
        parser.error('Credential plugins are not supported by this local-only launcher')
    access = urlparse(args.access_url)
    if access.scheme != 'http' or access.port != 3100 or not access.hostname or access.username or access.password:
        parser.error('--access-url must be an HTTP URL on port 3100 without credentials')
    if access.path not in ('', '/') or access.query or access.fragment:
        parser.error('--access-url must not contain a path, query, or fragment')

    # Do not inherit credentials or connection settings for another deployment.
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('CODER_', 'AWS_', 'TF_', 'KUBE_', 'PG'))
           and key not in ('KUBECONFIG', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY',
                           'http_proxy', 'https_proxy', 'all_proxy')}
    env.update({
        'DEVELOP_IN_CODER': '1',
        'AWS_CONFIG_FILE': os.devnull,
        'AWS_SHARED_CREDENTIALS_FILE': os.devnull,
        'AWS_EC2_METADATA_DISABLED': 'true',
        'TF_CLI_CONFIG_FILE': str(args.terraform_cli_config.expanduser().resolve(strict=True))
        if args.terraform_cli_config else os.devnull,
        'KUBE_CONFIG_PATH': str(kubeconfig),
        'KUBECONFIG': str(kubeconfig),
    })
    command = [
        './scripts/develop.sh', '--port', '3100', '--web-port', '8180',
        '--prometheus-port', '0', '--access-url', args.access_url, '--skip-setup', '--',
        '--experiments', 'network-policies', '--network-policy-module-path',
        str(root / 'scratch/coder-cilium/network-policy-module'),
        '--telemetry=false', '--derp-config-url', '', '--derp-server-stun-addresses', '',
        '--block-direct-connections=true',
    ]
    os.chdir(root)
    os.execve(command[0], command, env)


if __name__ == '__main__':
    main()
