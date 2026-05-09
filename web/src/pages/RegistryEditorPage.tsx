import { Link } from "react-router-dom";

export default function RegistryEditorPage() {
  return (
    <div className="py-12 text-center text-muted-foreground text-sm space-y-3">
      <p>Registry editor (stub — Task 10.6)</p>
      <Link to="/" className="text-primary hover:underline">
        ← Back to registry
      </Link>
    </div>
  );
}
