// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/logging/VictoriaLogsHook.java
// What: Hooks stdout/stderr and forwards lines to VictoriaLogs.
// Why: Capture logs without application changes.
package com.runtime.agent.logging;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;

public final class VictoriaLogsHook {
    private static volatile boolean installed = false;

    private VictoriaLogsHook() {}

    public static void install() {
        if (!VictoriaLogsSink.enabled()) {
            return;
        }
        if (System.out instanceof HookedPrintStream) {
            installed = true;
            return;
        }
        PrintStream originalOut = System.out;
        PrintStream originalErr = System.err;

        System.setOut(new HookedPrintStream(originalOut));
        System.setErr(new HookedPrintStream(originalErr));
        installed = true;
    }

    public static boolean isInstalled() {
        return installed;
    }

    private static final class HookedPrintStream extends PrintStream {
        private HookedPrintStream(PrintStream original) {
            super(new TeeOutputStream(original), true, StandardCharsets.UTF_8);
        }
    }

    private static final class TeeOutputStream extends OutputStream {
        private final OutputStream original;
        private final LineBuffer buffer = new LineBuffer();

        private TeeOutputStream(OutputStream original) {
            this.original = original;
        }

        @Override
        public void write(int b) throws IOException {
            original.write(b);
            buffer.write(b);
        }

        @Override
        public void write(byte[] b, int off, int len) throws IOException {
            original.write(b, off, len);
            buffer.write(b, off, len);
        }

        @Override
        public void flush() throws IOException {
            original.flush();
            buffer.flush();
        }
    }

    private static final class LineBuffer extends OutputStream {
        private final ByteArrayOutputStream buffer = new ByteArrayOutputStream();

        @Override
        public void write(int b) {
            if (b == '\n') {
                flushBuffer();
                return;
            }
            buffer.write(b);
        }

        @Override
        public void write(byte[] b, int off, int len) {
            for (int i = off; i < off + len; i++) {
                byte current = b[i];
                if (current == '\n') {
                    flushBuffer();
                } else {
                    buffer.write(current);
                }
            }
        }

        @Override
        public void flush() {
            flushBuffer();
        }

        private void flushBuffer() {
            if (buffer.size() == 0) {
                return;
            }
            String line = buffer.toString(StandardCharsets.UTF_8);
            buffer.reset();
            if (!line.isBlank()) {
                VictoriaLogsSink.sendLine(line);
            }
        }
    }
}
