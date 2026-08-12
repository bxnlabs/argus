import { useCallback } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { FileCode, Loader2 } from "lucide-react";

interface FileEditorProps {
  content: string;
  language: string;
  isBinary: boolean;
  isLarge: boolean;
}

export function FileEditor({ content, language, isBinary, isLarge }: FileEditorProps) {
  const handleMount: OnMount = useCallback((editor) => {
    editor.focus();
  }, []);

  if (isBinary) {
    return (
      <div className="text-muted-foreground flex h-full flex-col items-center justify-center p-8">
        <FileCode className="mb-4 h-12 w-12 opacity-50" />
        <p className="text-center text-sm">Binary file — cannot edit</p>
      </div>
    );
  }

  if (isLarge) {
    return (
      <div className="text-muted-foreground flex h-full flex-col items-center justify-center p-8">
        <FileCode className="mb-4 h-12 w-12 opacity-50" />
        <p className="text-center text-sm">File too large to display</p>
      </div>
    );
  }

  return (
    <Editor
      height="100%"
      language={language}
      value={content}
      theme="vs-dark"
      onMount={handleMount}
      loading={
        <div className="flex h-full items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      }
      options={{
        minimap: { enabled: false },
        fontSize: 13,
        fontFamily: "Menlo, Monaco, 'Courier New', monospace",
        lineNumbers: "on",
        scrollBeyondLastLine: false,
        wordWrap: "on",
        tabSize: 2,
        automaticLayout: true,
        bracketPairColorization: { enabled: true },
        guides: { bracketPairs: true },
        readOnly: true,
        // A viewer, not a disabled editor: no cursor line highlight or
        // blinking caret suggesting input is expected.
        renderLineHighlight: "none",
        cursorBlinking: "smooth",
        smoothScrolling: true,
        padding: { top: 8 },
      }}
    />
  );
}
