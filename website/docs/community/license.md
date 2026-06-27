# License, Forking & OpenTofu

azemu is free and open source. This page explains the licence, what you can
do with the code, how to fork it responsibly, and why we recommend running
it with [OpenTofu](https://opentofu.org).

## License

azemu is released under the **MIT License**. The full text lives in
[`LICENSE`](https://github.com/zerodeth/azemu/blob/main/LICENSE) at the root
of the repository.

In plain terms, MIT lets anyone:

- use azemu for any purpose, including commercially;
- copy, modify, and redistribute it;
- fork it and build a closed- or open-source product on top of it.

The only condition is **attribution**: the original copyright notice and the
MIT permission notice must be kept in all copies or substantial portions of
the software. There is no warranty.

!!! note "Not legal advice"
    This page summarises the licences involved to help you make decisions.
    It is not legal advice. When in doubt, read the licence texts and talk
    to a lawyer.

## Forking azemu

Forking is welcome and encouraged. If you fork azemu to take it in your own
direction, please do the following so users are not confused about who
maintains your copy:

1. **Keep the MIT notice.** Leave the original `LICENSE` file and copyright
   line intact, and add your own copyright line beneath it for your changes,
   for example:

    ```text
    Copyright (c) 2026 Sherif Abdalla
    Copyright (c) <year> <your name>
    ```

2. **Rename and rebrand.** Pick your own project name, module path, and
   binary name. Do not present your fork as the official azemu, and do not
   imply endorsement by the azemu maintainers. The name "azemu" and the
   project's logos are not part of the MIT grant; the licence covers the
   code, not the brand or trademarks.

3. **Update the module path.** Change `module github.com/zerodeth/azemu` in
   `go.mod` to your own repository path so imports resolve to your fork.

4. **Point links at your repo.** Update `repo_url`, `site_url`, issue links,
   and the support contact so reports reach you, not us.

5. **Contribute back when you can.** If your fork fixes a bug or adds a
   resource that fits azemu's scope, please open a pull request. Shared
   fidelity work benefits everyone in the ecosystem.

## Registering a forked or derived Terraform provider

azemu does **not** ship or fork a Terraform provider. It works with the
official, unmodified [`hashicorp/azurerm`](https://registry.terraform.io/providers/hashicorp/azurerm)
provider through the provider's built-in `metadata_host` field. See
[How It Works](../getting-started/how-it-works.md) for the redirect
mechanism. The same binary is published under the OpenTofu namespace as
[`opentofu/azurerm`](https://search.opentofu.org/provider/opentofu/azurerm/latest),
so nothing about azemu requires you to fork or republish a provider.

If your fork or downstream project *does* publish its own Terraform or
OpenTofu provider, register it under **your own namespace**:

- In the [Terraform Registry](https://registry.terraform.io) and the
  [OpenTofu Registry](https://search.opentofu.org), providers are namespaced
  by the publisher's organisation (for example `hashicorp/azurerm` or
  `opentofu/azurerm`). Publish under your own GitHub organisation
  (`your-org/your-provider`), not under `hashicorp/`, `opentofu/`, or
  `zerodeth/`.
- Provider source addresses in user configuration follow the same
  `registry.example.com/namespace/type` shape. Use a namespace you own.
- Sign your releases with your own GPG key and your own registry account.
  Do not reuse another publisher's signing identity or namespace.

This keeps provenance clear: users always know whose code they are running
and who to file issues against.

## Terraform and OpenTofu

azemu is an emulator of the Azure ARM API, not a Terraform distribution. It
works with both Terraform and OpenTofu because both speak the same provider
protocol and both honour the `metadata_host` redirect.

It is worth understanding how the two tools are licensed, because it affects
which one you can use freely:

| Tool | License | Open source? | Notes |
|------|---------|--------------|-------|
| Terraform `>= 1.6` | [BUSL 1.1](https://github.com/hashicorp/terraform/blob/main/LICENSE) | No (source-available) | HashiCorp relicensed Terraform from MPL 2.0 to the Business Source License in August 2023. BUSL restricts production use that competes with HashiCorp's offerings. |
| Terraform `< 1.6` | MPL 2.0 | Yes | The last fully open-source Terraform releases. |
| OpenTofu | [MPL 2.0](https://github.com/opentofu/opentofu/blob/main/LICENSE) | Yes | A drop-in fork of Terraform, governed by the Linux Foundation. Backward compatible with Terraform 1.6, no usage restrictions. |
| `azurerm` provider | MPL 2.0 | Yes | The provider itself did not change licence. It remains open source and is published to both registries. |

### Why we recommend OpenTofu

azemu's mission is a no-account, no-cost, fully open-source local Azure
loop. Pairing an MIT emulator with a source-available core tool would leave
a non-open-source link in that chain. OpenTofu keeps the whole toolchain
open:

- It is a true drop-in: swap the `terraform` command for `tofu`.
- It uses the same `azurerm` provider and the same HCL configuration.
- It carries no production-use restrictions.

azemu is tested against both, and we are not asking you to drop Terraform if
it works for you. We simply default our examples and recommendations to
OpenTofu so the open-source path is the obvious one.

```bash
# Terraform
terraform init && terraform apply -auto-approve

# OpenTofu (drop-in)
tofu init && tofu apply -auto-approve
```

Both run unchanged against azemu. See [Install](../getting-started/install.md)
for the full setup.

## Questions

Open a [discussion](https://github.com/zerodeth/azemu/discussions) or an
[issue](https://github.com/zerodeth/azemu/issues) if anything about
licensing, forking, or attribution is unclear. We would rather answer up
front than have you guess.
