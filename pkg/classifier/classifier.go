package classifier

import (
	"strings"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var domainKeywords = map[types.Domain][]string{
	types.DomainFrontend: {
		"react", "vue", "angular", "tailwind", "framer", "gsap", "anime", "css", "html",
		"design", "taste", "ui", "ux", "claymorphism", "glassmorphism", "neobrutalism",
		"shadcn", "remotion", "three-js", "canvas", "svg", "typography", "bento", "retro",
		"vintage", "matrix", "paper", "neon", "clean", "bold", "creative", "contemporary",
		"flat", "dithered", "doodle", "friendly", "gradient", "minimal", "mono", "pacman",
		"power", "premium", "refined", "riso", "roku", "sleek", "spacious", "square",
		"stitch", "tetris", "vibrant", "artistic", "basic", "cafe", "colorful", "corporate",
		"cosmic", "editorial", "fantasy", "futuristic", "material", "modern", "pneumorphism",
		"21st", "style", "carousel", "animation", "figma",
	},
	types.DomainBackend: {
		"api", "fastapi", "django", "flask", "spring", "aspnet", "dotnet", "laravel", "express",
		"hono", "koa", "graphql", "trpc", "sql", "postgres", "mysql", "prisma", "drizzle",
		"typeorm", "kysely", "supabase", "redis", "rabbitmq", "mqtt", "kafka", "mongodb",
		"grpc", "rest", "orm", "database", "c-sharp", "java", "go", "php", "rust", "elixir",
		"ruby", "python", "node", "deno", "bun", "backend",
	},
	types.DomainDevOps: {
		"docker", "kubernetes", "k8s", "terraform", "azure", "gcp", "google-cloud", "aws",
		"cloudflare", "helm", "istio", "service-mesh", "vercel", "netlify", "railway", "devops",
		"ci-cd", "github-actions", "gitlab", "homelab", "serverless", "workers", "monitoring",
		"prometheus", "grafana", "datadog", "infra", "deploy",
	},
	types.DomainOrchestration: {
		"agent", "kernel", "gsd", "fable", "sya", "ralph", "superpowers", "plan", "task",
		"orchestrat", "harness", "loop", "handoff", "delegate", "subagent", "team", "council",
		"reflex", "mythos", "context", "session", "prompt", "workstream", "workflow",
	},
	types.DomainTesting: {
		"test", "guard", "no-mistakes", "jest", "vitest", "playwright", "cypress", "pytest",
		"rspec", "tdd", "benchmark", "eval", "verify", "verification", "audit-uat", "smoke",
		"coverage", "selenium", "puppeteer", "qa", "check",
	},
	types.DomainSecurity: {
		"secur", "safety", "auth", "jwt", "gitleaks", "secret", "hipaa", "pci", "gdpr",
		"stride", "threat", "bounty", "sast", "penetration", "firewall", "hardening",
		"permission", "turnstile", "sandbox", "vulnerability",
	},
	types.DomainAI: {
		"deep-learning", "machine-learning", "pytorch", "jax", "transformers", "pandas",
		"numpy", "scipy", "scikit", "jupyter", "data-analyst", "data-science", "rag", "llm",
		"embedding", "vector", "fine-tuning", "biomolecular", "cheminformatics", "alphafold",
		"genome", "protein", "ncbi", "dbsnp", "clinvar", "chembl", "interpro", "pdb",
	},
	types.DomainReverseEng: {
		"rea", "xerj", "repowise", "debug", "reverse-engineer", "binary", "protocol", "frida",
		"ghidra", "rizin", "gdb", "packet", "wireshark", "ast-grep", "compiler", "profil",
		"disassembler", "trace", "decompile",
	},
	types.DomainContent: {
		"content", "marketing", "copywriting", "seo", "blog", "video", "elevenlabs", "podcast",
		"avatar", "audio", "tts", "stt", "social", "twitter", "linkedin", "image", "banner",
		"pitch-deck", "slides", "sales", "lead", "email", "press-release", "article",
	},
}

// ClassifyName maps an identifier or name into one of the 9 canonical domains.
func ClassifyName(name string) types.Domain {
	low := strings.ToLower(name)

	for domain, keywords := range domainKeywords {
		for _, kw := range keywords {
			if strings.Contains(low, kw) {
				return domain
			}
		}
	}

	return types.DomainGeneral
}
