from services.common.core.trace import TraceId


class TestTraceId:
    def test_generate_new_id(self):
        """新規Trace ID生成のフォーマット検証"""
        trace = TraceId.generate()
        trace_str = str(trace)

        # Format: Root=1-<8 hex>-<24 hex>;Sampled=1
        assert trace_str.startswith("Root=1-")

        parts = trace_str.split(";")
        root_part = parts[0]
        # Root=1-<8 hex>-<24 hex>
        root_val = root_part.split("=")[1]
        segments = root_val.split("-")

        assert len(segments) == 3
        assert segments[0] == "1"
        assert len(segments[1]) == 8  # epoch hex
        assert len(segments[2]) == 24  # unique id

    def test_parse_existing_header(self):
        """既存ヘッダーのパース検証"""
        header = "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8;Sampled=1"
        trace = TraceId.parse(header)

        assert trace.root == "1-5759e988-bd862e3fe1be46a994272793"
        assert trace.parent == "53995c3f42cd8ad8"
        assert trace.sampled == "1"
        assert str(trace) == header

    def test_parse_partial_header(self):
        """部分的なヘッダー（Rootのみ）のパース"""
        header = "Root=1-5759e988-bd862e3fe1be46a994272793"
        trace = TraceId.parse(header)
        assert trace.root == "1-5759e988-bd862e3fe1be46a994272793"
        assert trace.parent is None
        assert trace.sampled == "1"  # Default to 1

    def test_to_root_id(self):
        """ログ用の Root ID (Request ID) 取得"""
        header = "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8"
        trace = TraceId.parse(header)
        assert trace.to_root_id() == "1-5759e988-bd862e3fe1be46a994272793"
