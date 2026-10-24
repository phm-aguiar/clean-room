import sys


def apply(path, edits):
    with open(path, 'rb') as f:
        raw = f.read()
    text = raw.decode('utf-8')
    crlf = text.count('\r\n') * 2 > text.count('\n')
    for i, (old, new) in enumerate(edits):
        o = old
        n = new
        if crlf:
            o = o.replace('\n', '\r\n')
            n = n.replace('\n', '\r\n')
        c = text.count(o)
        if c != 1:
            print(f'EDIT {i} FAILED in {path}: count={c} old={old[:80]!r}')
            sys.exit(1)
        text = text.replace(o, n)
    with open(path, 'wb') as f:
        f.write(text.encode('utf-8'))
    print(f'{path}: {len(edits)} edits applied (crlf={crlf})')
