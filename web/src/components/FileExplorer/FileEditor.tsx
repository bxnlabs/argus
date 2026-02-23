import { useRef, useCallback } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { type editor } from "monaco-editor";
import { FileCode, Loader2 } from "lucide-react";

interface FileEditorProps {
  content: string;
  language: string;
  isBinary: boolean;
  isLarge: boolean;
  onChange: (content: string) => void;
  onSave?: () => void;
}

export function FileEditor({
  content,
  language,
  isBinary,
  isLarge,
  onChange,
  onSave,
}: FileEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const onSaveRef = useRef(onSave);
  onSaveRef.current = onSave;

  const handleMount: OnMount = useCallback((editor) => {
    editorRef.current = editor;

    // Register Cmd+S / Ctrl+S — uses ref so the binding stays current
    // even though onMount only fires once per editor instance.
    editor.addCommand(
      // Monaco.KeyMod.CtrlCmd | Monaco.KeyCode.KeyS
      2048 | 49, // CtrlCmd = 2048, KeyS = 49
      () => onSaveRef.current?.(),
    );

    // Focus editor
    editor.focus();
  }, []);

  const handleChange = useCallback(
    (value: string | undefined) => {
      if (value !== undefined) {
        onChange(value);
      }
    },
    [onChange],
  );

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
      onChange={handleChange}
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
        renderLineHighlight: "line",
        cursorBlinking: "smooth",
        smoothScrolling: true,
        padding: { top: 8 },
      }}
    />
  );
}
