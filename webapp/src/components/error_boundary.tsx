import React from 'react';

type Props = {
    area: string;
    children: React.ReactNode;
};

type State = {
    hasError: boolean;
    message: string;
};

const containerStyle: React.CSSProperties = {
    background: 'rgba(var(--error-text-color-rgb), 0.08)',
    border: '1px solid rgba(var(--error-text-color-rgb), 0.24)',
    borderRadius: '12px',
    color: 'var(--error-text)',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '16px',
};

export default class PluginErrorBoundary extends React.PureComponent<Props, State> {
    public state: State = {
        hasError: false,
        message: '',
    };

    public static getDerivedStateFromError(error: Error): State {
        return {
            hasError: true,
            message: error.message || '알 수 없는 오류가 발생했습니다.',
        };
    }

    public componentDidCatch(error: Error, info: React.ErrorInfo) {
        // eslint-disable-next-line no-console
        console.error(`[Upstage Parser] ${this.props.area} 렌더링 오류`, error, info);
    }

    public render() {
        if (this.state.hasError) {
            return (
                <div style={containerStyle}>
                    <strong>{`${this.props.area} 화면을 불러오지 못했습니다.`}</strong>
                    <span>{this.state.message}</span>
                    <span style={{fontSize: '12px', opacity: 0.85}}>
                        {'페이지를 새로고침한 뒤 다시 열어 보세요. 문제가 계속되면 플러그인 로그와 브라우저 콘솔을 함께 확인해 주세요.'}
                    </span>
                </div>
            );
        }

        return this.props.children;
    }
}
