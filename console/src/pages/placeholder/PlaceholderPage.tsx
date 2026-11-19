const PlaceholderPage = ({ title, description }: { title: string; description: string }) => (
  <section aria-labelledby="page-title" className="space-y-2">
    <h1 id="page-title" className="text-2xl font-semibold tracking-tight text-foreground">{title}</h1>
    <p className="text-sm text-muted-foreground">{description}</p>
    <div className="mt-6 rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
      This page is being built.
    </div>
  </section>
);

export default PlaceholderPage;
