import React, {useEffect, useRef, useState} from 'react';

import {renderMermaidDefinition} from '../mermaid_rendering';

type Props = {
    definition: string;
    postID: string;
    index: number;
};

export default function MermaidDiagram({definition, postID, index}: Props) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const popupContainerRef = useRef<HTMLDivElement | null>(null);
    const copyResetTimerRef = useRef<number | null>(null);
    const [error, setError] = useState('');
    const [popupError, setPopupError] = useState('');
    const [copied, setCopied] = useState(false);
    const [showSource, setShowSource] = useState(false);
    const [showRenderedPopup, setShowRenderedPopup] = useState(false);

    useEffect(() => {
        return renderIntoContainer({
            containerRef,
            definition,
            postID,
            index,
            setError,
            variant: 'inline',
        });
    }, [definition, index, postID]);

    useEffect(() => {
        if (!showRenderedPopup) {
            setPopupError('');
            if (popupContainerRef.current) {
                popupContainerRef.current.innerHTML = '';
            }
            return () => undefined;
        }

        return renderIntoContainer({
            containerRef: popupContainerRef,
            definition,
            postID,
            index,
            setError: setPopupError,
            variant: 'popup',
        });
    }, [definition, index, postID, showRenderedPopup]);

    useEffect(() => {
        return () => {
            if (copyResetTimerRef.current) {
                window.clearTimeout(copyResetTimerRef.current);
            }
        };
    }, []);

    const handleCopy = async () => {
        const copySucceeded = await copyText(definition);
        if (!copySucceeded) {
            setError((currentError) => currentError || '원문 복사에 실패했습니다. 브라우저 권한을 확인해 주세요.');
            return;
        }

        setCopied(true);
        if (copyResetTimerRef.current) {
            window.clearTimeout(copyResetTimerRef.current);
        }
        copyResetTimerRef.current = window.setTimeout(() => {
            setCopied(false);
        }, 1600);
    };

    return (
        <>
            <div className='upstage-mermaid-card'>
                <div className='upstage-mermaid-toolbar'>
                    <button
                        className='upstage-mermaid-toolbar-button'
                        onClick={handleCopy}
                        type='button'
                    >
                        {copied ? '복사됨' : '복사'}
                    </button>
                    <button
                        className='upstage-mermaid-toolbar-button'
                        onClick={() => setShowSource(true)}
                        type='button'
                    >
                        {'원문 보기'}
                    </button>
                    <button
                        className='upstage-mermaid-toolbar-button'
                        onClick={() => setShowRenderedPopup(true)}
                        type='button'
                    >
                        {'렌더 팝업'}
                    </button>
                </div>
                {error && (
                    <div className='upstage-mermaid-error'>
                        {`Mermaid 렌더링 실패: ${error}`}
                    </div>
                )}
                {error ? (
                    <div className='upstage-mermaid-fallback'>
                        <pre className='post-code'>
                            <code className='language-mermaid'>{definition}</code>
                        </pre>
                    </div>
                ) : (
                    <div className='upstage-mermaid-rendered'>
                        <div
                            data-testid='upstage-mermaid-diagram'
                            ref={containerRef}
                        />
                    </div>
                )}
            </div>
            {showRenderedPopup && (
                <div
                    className='upstage-mermaid-modal-backdrop'
                    onClick={() => setShowRenderedPopup(false)}
                    role='presentation'
                >
                    <div
                        className='upstage-mermaid-modal upstage-mermaid-render-modal'
                        onClick={(event) => event.stopPropagation()}
                        role='dialog'
                    >
                        <div className='upstage-mermaid-modal-header'>
                            <strong>{'Mermaid 렌더 팝업'}</strong>
                            <div className='upstage-mermaid-modal-actions'>
                                <button
                                    className='upstage-mermaid-toolbar-button'
                                    onClick={handleCopy}
                                    type='button'
                                >
                                    {copied ? '복사됨' : '복사'}
                                </button>
                                <button
                                    className='upstage-mermaid-toolbar-button'
                                    onClick={() => setShowRenderedPopup(false)}
                                    type='button'
                                >
                                    {'닫기'}
                                </button>
                            </div>
                        </div>
                        {popupError && (
                            <div className='upstage-mermaid-error upstage-mermaid-modal-error'>
                                {`Mermaid 렌더링 실패: ${popupError}`}
                            </div>
                        )}
                        <div className='upstage-mermaid-modal-content'>
                            {popupError ? (
                                <pre className='post-code upstage-mermaid-source'>
                                    <code className='language-mermaid'>{definition}</code>
                                </pre>
                            ) : (
                                <div
                                    className='upstage-mermaid-rendered upstage-mermaid-rendered-popup'
                                    data-testid='upstage-mermaid-diagram-popup'
                                    ref={popupContainerRef}
                                />
                            )}
                        </div>
                    </div>
                </div>
            )}
            {showSource && (
                <div
                    className='upstage-mermaid-modal-backdrop'
                    onClick={() => setShowSource(false)}
                    role='presentation'
                >
                    <div
                        className='upstage-mermaid-modal'
                        onClick={(event) => event.stopPropagation()}
                        role='dialog'
                    >
                        <div className='upstage-mermaid-modal-header'>
                            <strong>{'Mermaid 원문'}</strong>
                            <div className='upstage-mermaid-modal-actions'>
                                <button
                                    className='upstage-mermaid-toolbar-button'
                                    onClick={handleCopy}
                                    type='button'
                                >
                                    {copied ? '복사됨' : '복사'}
                                </button>
                                <button
                                    className='upstage-mermaid-toolbar-button'
                                    onClick={() => setShowSource(false)}
                                    type='button'
                                >
                                    {'닫기'}
                                </button>
                            </div>
                        </div>
                        <pre className='post-code upstage-mermaid-source'>
                            <code className='language-mermaid'>{definition}</code>
                        </pre>
                    </div>
                </div>
            )}
        </>
    );
}

async function copyText(value: string) {
    try {
        if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(value);
            return true;
        }
    } catch {
        return legacyCopy(value);
    }

    return legacyCopy(value);
}

function legacyCopy(value: string) {
    if (typeof document === 'undefined') {
        return false;
    }

    const textarea = document.createElement('textarea');
    textarea.value = value;
    textarea.setAttribute('readonly', 'true');
    textarea.style.opacity = '0';
    textarea.style.position = 'fixed';
    textarea.style.pointerEvents = 'none';
    document.body.appendChild(textarea);
    textarea.select();

    try {
        return document.execCommand('copy');
    } catch {
        return false;
    } finally {
        document.body.removeChild(textarea);
    }
}

type RenderIntoContainerOptions = {
    containerRef: React.RefObject<HTMLDivElement>;
    definition: string;
    postID: string;
    index: number;
    setError: React.Dispatch<React.SetStateAction<string>>;
    variant: string;
};

function renderIntoContainer({
    containerRef,
    definition,
    postID,
    index,
    setError,
    variant,
}: RenderIntoContainerOptions) {
    const container = containerRef.current;
    if (!container) {
        return () => undefined;
    }

    let cancelled = false;
    container.innerHTML = '';
    setError('');

    renderMermaidDefinition(definition, postID, index, variant).then(({svg, bindFunctions}) => {
        if (cancelled || !containerRef.current) {
            return;
        }
        containerRef.current.innerHTML = svg;
        bindFunctions?.(containerRef.current);
    }).catch((renderError: unknown) => {
        if (cancelled) {
            return;
        }
        const message = renderError instanceof Error ? renderError.message : String(renderError);
        setError(message);
    });

    return () => {
        cancelled = true;
        if (containerRef.current) {
            containerRef.current.innerHTML = '';
        }
    };
}
