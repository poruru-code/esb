"""
カスタム例外クラス

Lambda呼び出しに関するエラーを表現します。
"""


class LambdaInvokeError(Exception):
    """Lambda呼び出しの基底例外クラス"""

    pass


class FunctionNotFoundError(LambdaInvokeError):
    """関数が見つからない場合の例外"""

    def __init__(self, function_name: str):
        self.function_name = function_name
        super().__init__(f"Function not found: {function_name}")


class ContainerStartError(LambdaInvokeError):
    """コンテナ起動に失敗した場合の例外"""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause
        super().__init__(f"Failed to start container {function_name}: {cause}")


class LambdaExecutionError(LambdaInvokeError):
    """Lambda実行に失敗した場合の例外"""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause
        super().__init__(f"Lambda execution failed for {function_name}: {cause}")
