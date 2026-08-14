/** Placeholder rows shown while the editor chunk loads. */
export function EditorSkeleton() {
  return (
    <div className="flex h-full flex-col gap-2 p-4 pt-2">
      {[70, 45, 90, 60, 30, 80, 55, 40, 75, 50].map((w, i) => (
        <div key={i} className="flex items-center gap-3">
          <div className="bg-muted h-3 w-5 animate-pulse rounded" />
          <div className="bg-muted h-3 animate-pulse rounded" style={{ width: `${w}%` }} />
        </div>
      ))}
    </div>
  );
}
