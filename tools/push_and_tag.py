#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""用 GitHub Git Data API 推送改动并打 tag（无 gh CLI、避开 git/网页超时）。

用法：
    python push_and_tag.py 1.0.5 "v1.0.5：点 × 弹出选择框"
    python push_and_tag.py --tag-only 1.0.5 "备注"

做的事：
    1. 对比本地与远端 main 的 blob sha，只提交真正变了的文件（用 base_tree 合并）；
    2. PATCH refs/heads/main；
    3. 建 refs/tags/v<版本> 指向新提交 → 触发 GitHub Actions 的 tag 构建。
"""
import base64
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request

TOKEN = open('/opt/data/.secrets/github_douyin_token').read().strip()
OWNER, REPO = 'Tbe1998', 'pdf2img'
API = 'https://api.github.com'
ROOT = '/opt/data/projects/pdf2img'

SKIP_DIRS = {'.git', 'bin', 'testbin', '__pycache__', 'dist', 'design'}
SKIP_EXT = {'.zip', '.exe', '.syso', '.ico'}
SKIP_NAMES = {'latest.json', 'push_to_github.py', 'push_and_tag.py'}


def call(method, path, data=None):
    url = path if path.startswith('http') else API + path
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header('Authorization', 'token ' + TOKEN)
    req.add_header('Accept', 'application/vnd.github+json')
    req.add_header('User-Agent', 'hermes')
    if body:
        req.add_header('Content-Type', 'application/json')
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            txt = r.read()
            return r.status, (json.loads(txt) if txt else {})
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b'{}')


def blob_sha(path):
    data = open(path, 'rb').read()
    h = hashlib.sha1()
    h.update(b'blob %d\0' % len(data))
    h.update(data)
    return h.hexdigest(), data


def local_files():
    out = {}
    for dp, dn, fn in os.walk(ROOT):
        dn[:] = [d for d in dn if d not in SKIP_DIRS]
        for n in fn:
            if os.path.splitext(n)[1].lower() in SKIP_EXT or n in SKIP_NAMES:
                continue
            p = os.path.join(dp, n)
            out[os.path.relpath(p, ROOT).replace(os.sep, '/')] = p
    return out


def main():
    args = sys.argv[1:]
    tag_only = False
    if args and args[0] == '--tag-only':
        tag_only = True
        args = args[1:]
    if not args:
        print(__doc__)
        return 2
    version = args[0].lstrip('vV')
    note = args[1] if len(args) > 1 else 'v' + version

    st, repo = call('GET', f'/repos/{OWNER}/{REPO}')
    if st != 200:
        print('repo', st, repo)
        return 1
    branch = repo.get('default_branch') or 'main'
    st, ref = call('GET', f'/repos/{OWNER}/{REPO}/git/ref/heads/{branch}')
    base_sha = ref['object']['sha']
    st, base_commit = call('GET', f'/repos/{OWNER}/{REPO}/git/commits/{base_sha}')
    base_tree = base_commit['tree']['sha']

    new_commit = base_sha
    if not tag_only:
        st, rtree = call('GET', f'/repos/{OWNER}/{REPO}/git/trees/{base_tree}?recursive=1')
        remote = {x['path']: x['sha'] for x in rtree.get('tree', []) if x['type'] == 'blob'}
        changed = []
        for rel, p in sorted(local_files().items()):
            sha, _ = blob_sha(p)
            if remote.get(rel) != sha:
                changed.append(rel)
        if changed:
            print('变更文件:', changed)
            entries = []
            for rel in changed:
                data = open(os.path.join(ROOT, rel), 'rb').read()
                st, blob = call('POST', f'/repos/{OWNER}/{REPO}/git/blobs', {
                    'content': base64.b64encode(data).decode(), 'encoding': 'base64'})
                if st not in (200, 201):
                    print('blob fail', rel, st, blob)
                    return 1
                entries.append({'path': rel, 'mode': '100644', 'type': 'blob', 'sha': blob['sha']})
            st, t = call('POST', f'/repos/{OWNER}/{REPO}/git/trees',
                         {'base_tree': base_tree, 'tree': entries})
            if st not in (200, 201):
                print('tree', st, t)
                return 1
            st, c = call('POST', f'/repos/{OWNER}/{REPO}/git/commits',
                         {'message': note, 'tree': t['sha'], 'parents': [base_sha]})
            if st not in (200, 201):
                print('commit', st, c)
                return 1
            new_commit = c['sha']
            st, _ = call('PATCH', f'/repos/{OWNER}/{REPO}/git/refs/heads/{branch}',
                         {'sha': new_commit, 'force': False})
            if st not in (200, 201):
                print('update ref', st)
                return 1
            print('已提交', new_commit)
        else:
            print('没有变更，直接在当前 HEAD 上打 tag')

    tag = 'v' + version
    st, out = call('POST', f'/repos/{OWNER}/{REPO}/git/refs',
                   {'ref': 'refs/tags/' + tag, 'sha': new_commit})
    print('tag', tag, st, out.get('message', out.get('ref', '')))
    if st not in (200, 201):
        return 1
    print(f'https://github.com/{OWNER}/{REPO}/actions')
    return 0


if __name__ == '__main__':
    sys.exit(main())
