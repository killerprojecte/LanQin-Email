declare module "dompurify" {
  const DOMPurify: { sanitize: (source: string) => string }
  export default DOMPurify
}
