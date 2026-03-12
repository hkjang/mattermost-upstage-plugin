jest.mock('mermaid/dist/mermaid.esm.min.mjs', () => ({
    __esModule: true,
    default: {
        initialize: jest.fn(),
        render: jest.fn().mockResolvedValue({svg: '<svg></svg>'}),
    },
}));

import {containsCompleteMermaidFence, normalizeRenderableMessage, splitRenderableMessage} from './mermaid_rendering';

test('containsCompleteMermaidFence matches only closed mermaid fences', () => {
    expect(containsCompleteMermaidFence('```mermaid\ngraph TD\nA-->B\n```')).toBe(true);
    expect(containsCompleteMermaidFence('```mermaid\ngraph TD\nA-->B')).toBe(false);
});

test('splitRenderableMessage separates text and mermaid segments', () => {
    const segments = splitRenderableMessage([
        '서두 문장',
        '```mermaid',
        'graph TD',
        'A-->B',
        '```',
        '마무리 문장',
    ].join('\n'));

    expect(segments).toEqual([
        {kind: 'text', content: '서두 문장\n'},
        {kind: 'mermaid', content: 'graph TD\nA-->B'},
        {kind: 'text', content: '\n마무리 문장'},
    ]);
});

test('splitRenderableMessage keeps plain text when mermaid fence is incomplete', () => {
    const message = '```mermaid\ngraph TD\nA-->B';
    expect(splitRenderableMessage(message)).toEqual([{kind: 'text', content: message}]);
});

test('normalizeRenderableMessage removes blank lines inside markdown tables', () => {
    const normalized = normalizeRenderableMessage([
        '표입니다.',
        '',
        '    | 이름 | 값 |',
        '',
        '    | --- | --- |',
        '',
        '    | A | 1 |',
        '',
        '다음 문장',
    ].join('\n'));

    expect(normalized).toBe([
        '표입니다.',
        '',
        '| 이름 | 값 |',
        '| --- | --- |',
        '| A | 1 |',
        '',
        '다음 문장',
    ].join('\n'));
});

test('splitRenderableMessage detects indented mermaid fences after normalization', () => {
    const segments = splitRenderableMessage([
        '설명',
        '    ```mermaid',
        '    graph TD',
        '    A-->B',
        '    ```',
    ].join('\n'));

    expect(segments).toEqual([
        {kind: 'text', content: '설명\n'},
        {kind: 'mermaid', content: 'graph TD\nA-->B'},
    ]);
});

test('normalizeRenderableMessage removes one extra blank line inside fenced code blocks', () => {
    const normalized = normalizeRenderableMessage([
        '```python',
        '',
        'print("hello")',
        '',
        '```',
    ].join('\n'));

    expect(normalized).toBe([
        '```python',
        'print("hello")',
        '```',
    ].join('\n'));
});

test('normalizeRenderableMessage does not rewrite markdown table syntax inside fenced code blocks', () => {
    const normalized = normalizeRenderableMessage([
        '```text',
        '| 이름 | 값 |',
        '',
        '| --- | --- |',
        '```',
    ].join('\n'));

    expect(normalized).toBe([
        '```text',
        '| 이름 | 값 |',
        '',
        '| --- | --- |',
        '```',
    ].join('\n'));
});

test('normalizeRenderableMessage converts embedded html tables into markdown tables', () => {
    const normalized = normalizeRenderableMessage([
        '표 응답입니다.',
        '<table><tr><th>이름</th><th>값</th></tr><tr><td>A</td><td>1</td></tr></table>',
        '끝.',
    ].join('\n'));

    expect(normalized).toBe([
        '표 응답입니다.',
        '',
        '| 이름 | 값 |',
        '| --- | --- |',
        '| A | 1 |',
        '',
        '끝.',
    ].join('\n'));
});

test('normalizeRenderableMessage keeps html tables untouched inside fenced code blocks', () => {
    const normalized = normalizeRenderableMessage([
        '```html',
        '<table><tr><td>A</td></tr></table>',
        '```',
    ].join('\n'));

    expect(normalized).toBe([
        '```html',
        '<table><tr><td>A</td></tr></table>',
        '```',
    ].join('\n'));
});
