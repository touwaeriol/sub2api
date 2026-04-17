<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Editorial homepage -->
  <div
    v-else
    class="home-editorial min-h-screen bg-[#F5F4EE] text-[#1A1A1A] antialiased dark:bg-[#0F0E0B] dark:text-[#F5F4EE]"
  >
    <!-- Header -->
    <header class="sticky top-0 z-50 border-b border-[#1A1A1A]/10 bg-[#F5F4EE]/92 backdrop-blur-md dark:border-white/10 dark:bg-[#0F0E0B]/92">
      <nav class="mx-auto flex max-w-[1240px] items-center justify-between px-6 py-4 md:px-10">
        <router-link to="/" class="flex items-center" aria-label="Claude">
          <ClaudeLogo class="h-7 w-auto text-[#1A1A1A] dark:text-[#F5F4EE]" label="Claude" />
        </router-link>

        <div class="hidden items-center gap-7 md:flex">
          <a href="#capabilities" class="nav-link group">
            {{ t('home.nav.platform') }}
            <svg class="ml-1 h-3 w-3 opacity-55 transition-transform group-hover:translate-y-0.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
              <path d="M3 4.5l3 3 3-3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </a>
          <a href="#pricing" class="nav-link group">
            {{ t('home.nav.pricing') }}
            <svg class="ml-1 h-3 w-3 opacity-55 transition-transform group-hover:translate-y-0.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
              <path d="M3 4.5l3 3 3-3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </a>
          <router-link v-if="!docIsExternal" to="/docs" class="nav-link group">
            {{ t('home.nav.resources') }}
            <svg class="ml-1 h-3 w-3 opacity-55 transition-transform group-hover:translate-y-0.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
              <path d="M3 4.5l3 3 3-3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </router-link>
          <a v-else :href="docUrl" target="_blank" rel="noopener noreferrer" class="nav-link group">
            {{ t('home.nav.resources') }}
            <svg class="ml-1 h-3 w-3 opacity-55 transition-transform group-hover:translate-y-0.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true">
              <path d="M3 4.5l3 3 3-3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </a>
        </div>

        <div class="flex items-center gap-4">
          <div class="header-tools">
            <div class="locale-dd" ref="localeDdRef">
              <button
                type="button"
                class="locale-dd-trigger"
                :aria-expanded="localeOpen"
                aria-haspopup="listbox"
                @click="localeOpen = !localeOpen"
              >
                <span>{{ currentLocaleCode.toUpperCase() }}</span>
                <svg class="locale-dd-chevron" :class="{ 'is-open': localeOpen }" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M2.5 4l2.5 2.5L7.5 4"/>
                </svg>
              </button>
              <transition name="locale-pop">
                <div v-if="localeOpen" class="locale-dd-panel" role="listbox">
                  <button
                    v-for="l in availableLocales"
                    :key="l.code"
                    type="button"
                    class="locale-dd-option"
                    :class="{ 'is-active': l.code === currentLocaleCode }"
                    :disabled="switchingLocale"
                    role="option"
                    :aria-selected="l.code === currentLocaleCode"
                    @click="selectLocale(l.code)"
                  >
                    <span class="locale-dd-code">{{ l.code.toUpperCase() }}</span>
                    <span class="locale-dd-name">{{ l.name }}</span>
                    <svg v-if="l.code === currentLocaleCode" class="locale-dd-check" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                      <path d="M3.5 8.5l3 3 6-7"/>
                    </svg>
                  </button>
                </div>
              </transition>
            </div>
            <span class="header-tools-sep" aria-hidden="true"></span>
            <button
              type="button"
              class="theme-toggle"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              @click="toggleTheme"
            >
              <svg v-if="isDark" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <circle cx="8" cy="8" r="2.8"/>
                <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4"/>
              </svg>
              <svg v-else viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M13.2 9.3A5.6 5.6 0 0 1 6.7 2.8 5.6 5.6 0 1 0 13.2 9.3z"/>
              </svg>
            </button>
          </div>
          <router-link v-if="!docIsExternal" to="/docs" class="nav-link hidden sm:inline-block md:hidden">
            {{ t('docs.title') }}
          </router-link>
          <a v-else-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="nav-link hidden sm:inline-block md:hidden">
            {{ t('docs.title') }}
          </a>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="ce-btn-outline-sm"
          >
            <span class="hidden md:inline">{{ isAuthenticated ? t('home.dashboard') : t('home.nav.consoleLogin') }}</span>
            <span class="md:hidden">{{ isAuthenticated ? t('home.dashboard') : t('home.nav.login') }}</span>
            <span class="ml-0.5" aria-hidden>→</span>
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Hero -->
    <section id="top" class="px-6 pt-20 pb-24 md:px-10 md:pt-28 md:pb-32">
      <div class="mx-auto grid max-w-[1240px] grid-cols-1 items-center gap-16 md:grid-cols-12 md:gap-20">
        <!-- Left: headline + CTA -->
        <div class="md:col-span-5">
          <p class="eyebrow mb-6">— {{ heroEyebrow }}</p>
          <h1 class="font-display text-[48px] font-normal leading-[1.04] tracking-[-0.02em] md:text-[68px]">
            {{ heroTitle }}
          </h1>
          <p class="mt-8 max-w-[480px] text-[16px] leading-[1.55] text-[#1A1A1A]/65 dark:text-white/65 md:text-[17px]">
            {{ heroDescription }}
          </p>
          <p
            v-if="siteSubtitle"
            class="mt-3 max-w-[480px] text-[13px] text-[#1A1A1A]/45 dark:text-white/45"
          >
            {{ siteSubtitle }}
          </p>
          <div class="mt-10 flex flex-wrap items-center gap-x-6 gap-y-4">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/register'"
              class="ce-btn-primary ce-btn-lg"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <span aria-hidden>→</span>
            </router-link>
            <router-link v-if="!docIsExternal" to="/docs" class="ce-btn-link group">
              {{ t('home.viewDocs') }}
              <span class="transition-transform group-hover:translate-x-0.5" aria-hidden>→</span>
            </router-link>
            <a v-else :href="docUrl" target="_blank" rel="noopener noreferrer" class="ce-btn-link group">
              {{ t('home.viewDocs') }}
              <span class="transition-transform group-hover:translate-x-0.5" aria-hidden>→</span>
            </a>
          </div>
        </div>

        <!-- Right: minimal code card -->
        <div class="md:col-span-7">
          <figure class="code-card" aria-hidden="true">
            <figcaption class="code-card-label">
              <span class="code-card-file">messages.ts</span>
              <span class="code-card-lang">typescript</span>
            </figcaption>
            <pre class="code-card-body"><code><span class="ln"><span class="kw">const</span> claude = <span class="kw">new</span> Anthropic({</span>
<span class="ln">  baseURL: <span class="str">"{{ apiBaseUrl }}"</span>,</span>
<span class="ln">});</span>
<span class="ln"> </span>
<span class="ln"><span class="kw">const</span> reply = <span class="kw">await</span> claude.messages.create({</span>
<span class="ln">  model: <span class="str">"claude-opus-4-6"</span>,</span>
<span class="ln">  messages: [{ role: <span class="str">"user"</span>, content: <span class="str">"Hi"</span> }],</span>
<span class="ln">});</span></code></pre>
          </figure>
        </div>
      </div>
    </section>

    <!-- Trust / SDK compatibility row -->
    <section class="border-t border-[#1A1A1A]/10 dark:border-white/10">
      <div class="mx-auto flex max-w-[1240px] flex-wrap items-center justify-center gap-x-10 gap-y-3 px-6 py-5 md:px-10">
        <span
          v-for="label in trustItems"
          :key="label"
          class="inline-flex items-center gap-2 text-[12px] text-[#1A1A1A]/60 dark:text-white/60"
        >
          <svg class="h-3.5 w-3.5 text-[#CC785C]" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M3.5 8.5l3 3 6-7"/>
          </svg>
          {{ label }}
        </span>
      </div>
    </section>

    <!-- Compatible with strip -->
    <section class="border-y border-[#1A1A1A]/10 dark:border-white/10">
      <div class="mx-auto flex max-w-[1200px] flex-col items-start gap-7 px-6 py-12 md:flex-row md:items-center md:justify-between md:gap-10 md:px-10">
        <p class="eyebrow shrink-0">{{ t('home.eyebrow.compatible') }}</p>
        <ul class="flex flex-wrap items-center gap-x-10 gap-y-3 font-display text-[20px] text-[#1A1A1A]/75 dark:text-white/75 md:text-[22px]">
          <li>{{ t('home.providers.claude') }}</li>
          <li aria-hidden class="text-[#1A1A1A]/15 dark:text-white/15">·</li>
          <li>GPT</li>
          <li aria-hidden class="text-[#1A1A1A]/15 dark:text-white/15">·</li>
          <li>{{ t('home.providers.gemini') }}</li>
          <li aria-hidden class="text-[#1A1A1A]/15 dark:text-white/15">·</li>
          <li>{{ t('home.providers.antigravity') }}</li>
          <li aria-hidden class="text-[#1A1A1A]/15 dark:text-white/15">·</li>
          <li class="text-[#1A1A1A]/35 dark:text-white/35">{{ t('home.providers.more') }}</li>
        </ul>
      </div>
    </section>

    <!-- Why choose -->
    <section id="capabilities" class="border-t border-[#1A1A1A]/10 px-6 py-28 md:px-10 md:py-36 dark:border-white/10">
      <div class="mx-auto max-w-[1240px]">
        <div class="max-w-[760px]">
          <p class="eyebrow mb-5">— {{ whyChooseEyebrow }}</p>
          <h2 class="font-display text-[38px] leading-[1.06] tracking-[-0.018em] md:text-[56px]">
            {{ whyChooseTitle }}
          </h2>
          <p class="mt-6 max-w-[520px] text-[16px] leading-[1.65] text-[#1A1A1A]/65 dark:text-white/65">
            {{ whyChooseSubtitle }}
          </p>
        </div>

        <div class="mt-16 grid grid-cols-1 gap-px bg-[#1A1A1A]/10 md:grid-cols-2 lg:grid-cols-3 dark:bg-white/10">
          <article
            v-for="(item, idx) in whyChooseList"
            :key="idx"
            class="bg-[#F5F4EE] p-8 transition-colors hover:bg-[#F1EFE6] md:p-10 dark:bg-[#0F0E0B] dark:hover:bg-[#15130E]"
          >
            <div class="mb-7 flex h-11 w-11 items-center justify-center rounded-full border border-[#CC785C]/25 bg-[#CC785C]/8 text-[#CC785C]">
              <Icon :name="(item.icon as any)" size="md" :stroke-width="1.6" />
            </div>
            <h3 class="font-display text-[22px] leading-tight tracking-tight md:text-[24px]">
              {{ item.title }}
            </h3>
            <p class="mt-3 text-[14px] leading-[1.65] text-[#1A1A1A]/65 dark:text-white/65">
              {{ item.desc }}
            </p>
          </article>
        </div>
      </div>
    </section>

    <!-- Model Channels (pricing) -->
    <section id="pricing" class="border-t border-[#1A1A1A]/10 px-6 py-28 md:px-10 md:py-36 dark:border-white/10">
      <div class="mx-auto max-w-[1240px]">
        <div class="max-w-[760px]">
          <p class="eyebrow mb-5">— {{ channelsEyebrow }}</p>
          <h2 class="font-display text-[38px] leading-[1.06] tracking-[-0.018em] md:text-[56px]">
            {{ channelsTitle }}
          </h2>
          <p class="mt-6 max-w-[560px] text-[16px] leading-[1.65] text-[#1A1A1A]/65 dark:text-white/65">
            {{ channelsSubtitle }}
          </p>
        </div>

        <div class="mt-16 grid grid-cols-1 gap-6 md:grid-cols-3">
          <article
            v-for="channel in channelsList"
            :key="channel.name"
            class="channel-card"
            :class="{ 'channel-card-highlight': channel.highlight }"
          >
            <div class="channel-icon" v-html="channel.icon"></div>

            <h3 class="channel-name font-display">{{ channel.name }}</h3>
            <p class="channel-tagline">{{ channel.tagline }}</p>

            <div class="channel-price">
              <div class="channel-price-row">
                <span class="font-display tabular-nums">1 : {{ channel.rate }}</span>
                <span class="channel-price-label">{{ t('home.channels.currentRate') }}</span>
              </div>
              <p class="channel-credit">{{ t('home.channels.creditLine', { usd: channel.credit }) }}</p>
            </div>

            <a
              v-if="purchaseIsExternal"
              :href="purchaseExternalHref"
              target="_blank"
              rel="noopener noreferrer"
              class="channel-cta"
            >
              {{ t('home.channels.cta') }}
            </a>
            <router-link
              v-else
              :to="purchaseInternalRoute"
              class="channel-cta"
            >
              {{ t('home.channels.cta') }}
            </router-link>
            <p class="channel-fineprint">{{ t('home.channels.fineprint') }}</p>

            <div class="channel-divider"></div>

            <p class="channel-section-title">
              {{ t('home.channels.supportedModels') }}
            </p>
            <ul class="channel-list">
              <li v-for="m in channel.models.slice(0, 4)" :key="m">
                <svg class="check" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7"/></svg>
                <span class="font-mono">{{ m }}</span>
              </li>
              <li v-if="channel.models.length > 4" class="channel-list-more">
                {{ t('home.channels.andMore', { n: channel.models.length - 4 }) }}
              </li>
            </ul>

            <p class="channel-section-title mt-6">{{ t('home.channels.features') }}</p>
            <ul class="channel-list">
              <li v-for="f in channel.features" :key="f">
                <svg class="check" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7"/></svg>
                <span>{{ f }}</span>
              </li>
            </ul>
          </article>
        </div>

      </div>
    </section>

    <!-- FAQ -->
    <section id="faq" class="border-t border-[#1A1A1A]/10 px-6 py-28 md:px-10 md:py-36 dark:border-white/10">
      <div class="mx-auto grid max-w-[1200px] grid-cols-1 gap-16 md:grid-cols-12">
        <div class="md:col-span-4">
          <p class="eyebrow mb-5">— {{ faqEyebrow }}</p>
          <h2 class="font-display text-[38px] leading-[1.06] tracking-[-0.018em] md:text-[56px]">
            {{ faqTitle }}
          </h2>
          <p class="mt-6 max-w-[360px] text-[15px] leading-[1.65] text-[#1A1A1A]/65 dark:text-white/65">
            {{ faqSubtitle }}
          </p>
        </div>
        <div class="md:col-span-8">
          <div class="divide-y divide-[#1A1A1A]/10 border-y border-[#1A1A1A]/10 dark:divide-white/10 dark:border-white/10">
            <div
              v-for="(f, idx) in faqList"
              :key="idx"
              class="py-2"
            >
              <button
                type="button"
                class="flex w-full items-start justify-between gap-6 py-6 text-left"
                :aria-expanded="openFaq === idx"
                @click="toggleFaq(idx)"
              >
                <h3 class="font-display text-[22px] leading-tight tracking-tight transition md:text-[24px]" :class="openFaq === idx ? 'text-[#CC785C]' : ''">
                  {{ f.q }}
                </h3>
                <span
                  class="mt-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-[#1A1A1A]/15 text-[#1A1A1A]/60 transition dark:border-white/15 dark:text-white/60"
                  :class="openFaq === idx ? 'rotate-180 border-[#CC785C] bg-[#CC785C]/8 text-[#CC785C]' : ''"
                  aria-hidden="true"
                >
                  <svg class="h-3.5 w-3.5" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.8">
                    <path d="M3.5 5.5L7 9l3.5-3.5" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </span>
              </button>
              <div
                v-show="openFaq === idx"
                class="pb-7 pr-12"
              >
                <p class="max-w-[620px] text-[15px] leading-[1.75] text-[#1A1A1A]/70 dark:text-white/70">
                  {{ f.a }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Comparison / CTA -->
    <section class="border-t border-[#1A1A1A]/10 px-6 py-28 md:px-10 md:py-36 dark:border-white/10">
      <div class="mx-auto max-w-[1200px]">
        <div class="max-w-[720px]">
          <p class="eyebrow mb-5">— {{ comparisonEyebrow }}</p>
          <h2 class="font-display text-[38px] leading-[1.06] tracking-[-0.018em] md:text-[56px]">
            {{ comparisonTitle }}
          </h2>
          <p class="mt-6 max-w-[560px] text-[16px] leading-[1.65] text-[#1A1A1A]/65 dark:text-white/65">
            {{ comparisonBody }}
          </p>
        </div>

        <div class="mt-16 border-t border-[#1A1A1A]/12 dark:border-white/12">
          <!-- Header row -->
          <div class="grid grid-cols-12 gap-6 border-b border-[#1A1A1A]/12 py-5 dark:border-white/12">
            <div class="col-span-4 text-[11px] font-medium uppercase tracking-[0.16em] text-[#1A1A1A]/45 dark:text-white/45">{{ comparisonHeaderFeature }}</div>
            <div class="col-span-4 text-[11px] font-medium uppercase tracking-[0.16em] text-[#1A1A1A]/45 dark:text-white/45">{{ comparisonHeaderOfficial }}</div>
            <div class="col-span-4 text-[11px] font-medium uppercase tracking-[0.16em] text-[#CC785C]">{{ comparisonHeaderUs }}</div>
          </div>
          <div
            v-for="(row, idx) in comparisonList"
            :key="idx"
            class="grid grid-cols-12 items-start gap-6 border-b border-[#1A1A1A]/10 py-7 dark:border-white/10"
          >
            <div class="col-span-4 font-display text-[18px] leading-tight tracking-tight md:text-[20px]">
              {{ row.feature }}
            </div>
            <div class="col-span-4 flex items-start gap-2 text-[14px] leading-[1.55] text-[#1A1A1A]/50 dark:text-white/50">
              <svg class="mt-1 h-3.5 w-3.5 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>
              <span>{{ row.official }}</span>
            </div>
            <div class="col-span-4 flex items-start gap-2 text-[14px] leading-[1.55] text-[#1A1A1A]/85 dark:text-white/85">
              <svg class="mt-1 h-3.5 w-3.5 shrink-0 text-[#CC785C]" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7"/></svg>
              <span>{{ row.us }}</span>
            </div>
          </div>
        </div>

        <div class="mt-14 flex flex-wrap items-center justify-center gap-x-8 gap-y-4">
          <router-link
            :to="isAuthenticated ? dashboardPath : '/register'"
            class="ce-btn-primary ce-btn-lg"
          >
            {{ t('home.cta.button') }}
            <span aria-hidden>→</span>
          </router-link>
          <router-link
            v-if="!docIsExternal"
            to="/docs"
            class="ce-btn-link group"
          >
            {{ t('home.viewDocs') }}
            <span class="transition-transform group-hover:translate-x-0.5" aria-hidden>→</span>
          </router-link>
          <a v-else :href="docUrl" target="_blank" rel="noopener noreferrer" class="ce-btn-link group">
            {{ t('home.viewDocs') }}
            <span class="transition-transform group-hover:translate-x-0.5" aria-hidden>→</span>
          </a>
        </div>
      </div>
    </section>

    <!-- Footer -->
    <footer class="border-t border-[#1A1A1A]/10 px-6 py-16 md:px-10 dark:border-white/10">
      <div class="mx-auto max-w-[1200px]">
        <div class="grid grid-cols-2 gap-10 md:grid-cols-4">
          <div class="col-span-2">
            <router-link to="/" class="inline-flex items-center" aria-label="Claude">
              <ClaudeLogo class="h-6 w-auto text-[#1A1A1A] dark:text-[#F5F4EE]" label="Claude" />
            </router-link>
            <p class="mt-5 max-w-[260px] text-[13px] leading-[1.65] text-[#1A1A1A]/55 dark:text-white/55">
              {{ t('home.heroDescription') }}
            </p>
          </div>
          <div>
            <h4 class="eyebrow text-[#1A1A1A]/55 dark:text-white/55">{{ t('home.footer.product') }}</h4>
            <ul class="mt-5 space-y-3">
              <li><a href="#capabilities" class="footer-link">{{ t('home.nav.platform') }}</a></li>
              <li><a href="#pricing" class="footer-link">{{ t('home.nav.pricing') }}</a></li>
              <li><a href="#faq" class="footer-link">{{ t('home.faq.eyebrow') }}</a></li>
            </ul>
          </div>
          <div>
            <h4 class="eyebrow text-[#1A1A1A]/55 dark:text-white/55">{{ t('home.footer.support') }}</h4>
            <ul class="mt-5 space-y-3">
              <li>
                <router-link v-if="!docIsExternal" to="/docs" class="footer-link">{{ t('home.nav.resources') }}</router-link>
                <a v-else :href="docUrl" target="_blank" rel="noopener noreferrer" class="footer-link">{{ t('home.nav.resources') }}</a>
              </li>
              <li v-if="!isAuthenticated">
                <router-link to="/login" class="footer-link">{{ t('home.nav.consoleLogin') }}</router-link>
              </li>
            </ul>
          </div>
        </div>
        <div class="mt-16 flex flex-col items-start justify-between gap-3 border-t border-[#1A1A1A]/10 pt-8 md:flex-row md:items-center dark:border-white/10">
          <p class="text-[12px] text-[#1A1A1A]/50 dark:text-white/50">
            &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
          </p>
          <p class="text-[12px] italic text-[#1A1A1A]/45 dark:text-white/45">
            {{ t('home.heroSubtitle') }}
          </p>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import ClaudeLogo from '@/components/icons/ClaudeLogo.vue'
import { setLocale, availableLocales } from '@/i18n'

const { t, locale } = useI18n()

const currentLocaleCode = computed(() => locale.value)
const switchingLocale = ref(false)
const localeOpen = ref(false)
const localeDdRef = ref<HTMLElement | null>(null)
async function selectLocale(code: string) {
  if (switchingLocale.value) return
  if (code === currentLocaleCode.value) {
    localeOpen.value = false
    return
  }
  switchingLocale.value = true
  try {
    await setLocale(code)
  } finally {
    switchingLocale.value = false
    localeOpen.value = false
  }
}
function handleLocaleClickOutside(event: MouseEvent) {
  if (localeDdRef.value && !localeDdRef.value.contains(event.target as Node)) {
    localeOpen.value = false
  }
}

const authStore = useAuthStore()
const appStore = useAppStore()

const rawSiteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Claude')
const rawSiteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
// Brand strings: locale i18n wins, admin settings fall back.
// Lets the homepage show "anthropic.mom" in EN while Chinese users still see
// whatever the admin set (e.g. "A 社妈妈" / "中转的上游渠道").
function resolveBrand(i18nKey: string, fallback: string): string {
  const translated = t(i18nKey)
  if (translated && translated !== i18nKey && translated.trim() !== '') {
    return translated
  }
  return fallback
}
const siteName = computed(() => resolveBrand('brand.name', rawSiteName.value))
const siteSubtitle = computed(() => resolveBrand('brand.tagline', rawSiteSubtitle.value))
const externalDocUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const docUrl = computed(() => externalDocUrl.value || '/docs')
const docIsExternal = computed(() => !!externalDocUrl.value)
const apiBaseUrl = computed(() => {
  const raw = appStore.cachedPublicSettings?.api_base_url || ''
  if (raw) return raw.replace(/\/$/, '')
  // Fallback: use current origin when running in browser
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin
  }
  return 'https://your-gateway.com'
})

// Contact link — admin `contact_info` accepts a mailto:, https://, plain email,
// or arbitrary text. We only link the first three; plain text falls back to
// the default mailto.
// Purchase CTA — if admin enables `purchase_subscription_url`, the channel
// cards link out to the external purchase page; otherwise they stay on the
// internal /purchase or /register route.
// NOTE: `purchase_subscription_*` fields exist in the backend response but
// the upstream-owned `PublicSettings` type hasn't declared them yet, so we
// read them via a widened cast to avoid touching upstream types.
type PurchaseSettings = { purchase_subscription_enabled?: boolean; purchase_subscription_url?: string }
const purchaseSettings = computed<PurchaseSettings>(() => (appStore.cachedPublicSettings as PurchaseSettings | null) ?? {})
const purchaseIsExternal = computed(() => Boolean(purchaseSettings.value.purchase_subscription_enabled && purchaseSettings.value.purchase_subscription_url))
const purchaseExternalHref = computed(() => purchaseSettings.value.purchase_subscription_url || '')
const purchaseInternalRoute = computed(() => (isAuthenticated.value ? '/purchase' : '/register'))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')

const currentYear = computed(() => new Date().getFullYear())

// ============================================================================
// home_config: admin-editable JSON blob with optional multi-language fields
// ============================================================================
// Schema (all fields optional — missing ones fall back to the i18n defaults):
//   {
//     "hero":   { "eyebrow": L, "title": L, "description": L, "rightTitle": L },
//     "trust":  [ L, L, L ],
//     "whyChoose": {
//       "eyebrow": L, "title": L, "subtitle": L,
//       "items": [ { "icon": "dollar", "title": L, "desc": L }, ... ]
//     },
//     "channels": {
//       "eyebrow": L, "title": L, "subtitle": L,
//       "items": [
//         { "name": "AWS Bedrock", "badge": "Claude", "tagline": L,
//           "rate": "5", "credit": "0.20", "desc": L,
//           "models": ["claude-opus-4-6 (1M)", ...],
//           "features": [ L, L ],
//           "icon": "bedrock" | "max" | "orbit",
//           "highlight": false }
//       ]
//     },
//     "comparison": {
//       "eyebrow": L, "title": L, "body": L,
//       "headers": { "feature": L, "official": L, "us": L },
//       "items": [ { "feature": L, "official": L, "us": L }, ... ]
//     },
//     "faq": {
//       "eyebrow": L, "title": L, "subtitle": L,
//       "items": [ { "q": L, "a": L }, ... ]
//     }
//   }
//
// Where L is `string | { en: string, zh: string, ... }`. A bare string is
// treated as locale-neutral. An object is keyed by locale code with `en`
// then `zh` as ultimate fallbacks.

type Localized = string | Record<string, string> | undefined | null
type HomeConfigDoc = {
  hero?: { eyebrow?: Localized; title?: Localized; description?: Localized; rightTitle?: Localized }
  trust?: Localized[]
  whyChoose?: {
    eyebrow?: Localized; title?: Localized; subtitle?: Localized
    items?: Array<{ icon?: string; title?: Localized; desc?: Localized }>
  }
  channels?: {
    eyebrow?: Localized; title?: Localized; subtitle?: Localized
    items?: Array<{
      name?: string; badge?: string; tagline?: Localized
      rate?: string; credit?: string; desc?: Localized
      models?: string[]; features?: Localized[]
      icon?: string; highlight?: boolean
    }>
  }
  comparison?: {
    eyebrow?: Localized; title?: Localized; body?: Localized
    headers?: { feature?: Localized; official?: Localized; us?: Localized }
    items?: Array<{ feature?: Localized; official?: Localized; us?: Localized }>
  }
  faq?: {
    eyebrow?: Localized; title?: Localized; subtitle?: Localized
    items?: Array<{ q?: Localized; a?: Localized }>
  }
}

function localized(value: Localized, loc: string, fallback = ''): string {
  if (value == null) return fallback
  if (typeof value === 'string') return value || fallback
  if (typeof value === 'object') {
    const v = (value[loc] ?? value.en ?? value.zh)
    return typeof v === 'string' && v.trim() !== '' ? v : fallback
  }
  return fallback
}

const homeConfig = computed<HomeConfigDoc | null>(() => {
  // home_config is a new key not yet declared on the upstream PublicSettings type.
  // Read via a widened cast to avoid touching upstream-owned types.
  const settings = appStore.cachedPublicSettings as { home_config?: string } | null
  const raw = (settings?.home_config || '').trim()
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') return parsed as HomeConfigDoc
  } catch (e) {
    if (typeof console !== 'undefined') {
      console.warn('[home] home_config JSON parse failed, falling back to defaults:', e)
    }
  }
  return null
})

// Convenience: current locale string for `localized()` calls.
const loc = computed(() => locale.value as string)

// ============================================================================
// Hero / trust / whyChoose / channels / comparison / faq computeds.
// Each one prefers the admin home_config value, falling back to the existing
// i18n key. Static defaults below are still used when admin hasn't customized.
// ============================================================================

const heroEyebrow = computed(() => localized(homeConfig.value?.hero?.eyebrow, loc.value, t('home.eyebrow.platform')))
const heroTitle = computed(() => localized(homeConfig.value?.hero?.title, loc.value, t('home.heroSubtitle')))
const heroDescription = computed(() => localized(homeConfig.value?.hero?.description, loc.value, t('home.heroDescription')))

const defaultTrust = computed(() => [t('home.trust.sdk'), t('home.trust.openai'), t('home.trust.dropin')])
const trustItems = computed(() => {
  const cfg = homeConfig.value?.trust
  if (Array.isArray(cfg) && cfg.length > 0) {
    return cfg.map((v, i) => localized(v, loc.value, defaultTrust.value[i] || ''))
  }
  return defaultTrust.value
})

const whyChooseEyebrow = computed(() => localized(homeConfig.value?.whyChoose?.eyebrow, loc.value, t('home.whyChoose.eyebrow')))
const whyChooseTitle = computed(() => localized(homeConfig.value?.whyChoose?.title, loc.value, t('home.whyChoose.title')))
const whyChooseSubtitle = computed(() => localized(homeConfig.value?.whyChoose?.subtitle, loc.value, t('home.whyChoose.subtitle')))

const channelsEyebrow = computed(() => localized(homeConfig.value?.channels?.eyebrow, loc.value, t('home.channels.eyebrow')))
const channelsTitle = computed(() => localized(homeConfig.value?.channels?.title, loc.value, t('home.channels.title')))
const channelsSubtitle = computed(() => localized(homeConfig.value?.channels?.subtitle, loc.value, t('home.channels.subtitle')))

const comparisonEyebrow = computed(() => localized(homeConfig.value?.comparison?.eyebrow, loc.value, t('home.comparison.title')))
const comparisonTitle = computed(() => localized(homeConfig.value?.comparison?.title, loc.value, t('home.editorial.ctaTitle')))
const comparisonBody = computed(() => localized(homeConfig.value?.comparison?.body, loc.value, t('home.editorial.ctaBody')))
const comparisonHeaderFeature = computed(() => localized(homeConfig.value?.comparison?.headers?.feature, loc.value, t('home.comparison.headers.feature')))
const comparisonHeaderOfficial = computed(() => localized(homeConfig.value?.comparison?.headers?.official, loc.value, t('home.comparison.headers.official')))
const comparisonHeaderUs = computed(() => localized(homeConfig.value?.comparison?.headers?.us, loc.value, t('home.comparison.headers.us')))

const faqEyebrow = computed(() => localized(homeConfig.value?.faq?.eyebrow, loc.value, t('home.faq.eyebrow')))
const faqTitle = computed(() => localized(homeConfig.value?.faq?.title, loc.value, t('home.faq.title')))
const faqSubtitle = computed(() => localized(homeConfig.value?.faq?.subtitle, loc.value, t('home.faq.subtitle')))

// Edit these arrays to change home content — they intentionally live in code,
// not in the admin settings, so merges with upstream stay simple.
const whyChooseItems = [
  { icon: 'dollar', titleKey: 'home.whyChoose.items.pricing.title', descKey: 'home.whyChoose.items.pricing.desc' },
  { icon: 'shield', titleKey: 'home.whyChoose.items.stable.title', descKey: 'home.whyChoose.items.stable.desc' },
  { icon: 'bolt', titleKey: 'home.whyChoose.items.instant.title', descKey: 'home.whyChoose.items.instant.desc' },
  { icon: 'cube', titleKey: 'home.whyChoose.items.models.title', descKey: 'home.whyChoose.items.models.desc' },
  { icon: 'clock', titleKey: 'home.whyChoose.items.latency.title', descKey: 'home.whyChoose.items.latency.desc' },
  { icon: 'chat', titleKey: 'home.whyChoose.items.support.title', descKey: 'home.whyChoose.items.support.desc' }
] as const

type Channel = {
  name: string
  badge: string
  tagline: string
  rate: string
  credit: string
  desc: string
  models: string[]
  features: string[]
  icon: string
  highlight?: boolean
}
// Line-art SVG marks (kept inline as strings so the channels array is self-contained).
const ICON_BEDROCK = `<svg viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M8 16h32M8 24h32M8 32h32"/><path d="M14 10v6M24 10v6M34 10v6" opacity="0.5"/><path d="M14 32v6M24 32v6M34 32v6" opacity="0.5"/><path d="M10 19l4-1M20 19l4-1M30 19l4-1" opacity="0.35"/><path d="M10 27l4-1M20 27l4-1M30 27l4-1" opacity="0.35"/></svg>`
const ICON_MAX = `<svg viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M24 4v8M24 36v8M4 24h8M36 24h8"/><path d="M10 10l5.5 5.5M32.5 32.5L38 38M38 10l-5.5 5.5M15.5 32.5L10 38" opacity="0.75"/><circle cx="24" cy="24" r="6"/><circle cx="24" cy="24" r="2" fill="currentColor" stroke="none"/></svg>`
const ICON_ORBIT = `<svg viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><circle cx="24" cy="24" r="18" opacity="0.3"/><ellipse cx="24" cy="24" rx="18" ry="6" transform="rotate(-28 24 24)"/><ellipse cx="24" cy="24" rx="18" ry="6" transform="rotate(28 24 24)" opacity="0.5"/><circle cx="24" cy="24" r="2.5" fill="currentColor" stroke="none"/></svg>`
// ICON_OPENAI: 三条 60° 旋转的椭圆组成的六瓣花形，是 OpenAI blossom 的简化线稿版。
const ICON_OPENAI = `<svg viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="24" cy="24" rx="14" ry="6"/><ellipse cx="24" cy="24" rx="14" ry="6" transform="rotate(60 24 24)"/><ellipse cx="24" cy="24" rx="14" ry="6" transform="rotate(120 24 24)"/><circle cx="24" cy="24" r="2" fill="currentColor" stroke="none"/></svg>`

const channels: Channel[] = [
  {
    name: 'AWS Bedrock',
    badge: 'Claude',
    tagline: 'Enterprise Claude via AWS',
    rate: '5',
    credit: '0.20',
    desc: '基于 AWS 的 Claude 企业级接入服务,适合对稳定性要求高的生产环境。',
    models: ['claude-opus-4-6 (1M)', 'claude-opus-4-5', 'claude-sonnet-4-6', 'claude-sonnet-4-5', 'claude-haiku-4-5'],
    features: ['AWS 官方托管', '200k 上下文', '支持 WebSearch', '相当于官方 7 折'],
    icon: ICON_BEDROCK
  },
  {
    name: 'Claude Max',
    badge: 'Claude',
    tagline: 'Full throughput, full context',
    rate: '2.1',
    credit: '0.48',
    desc: '官方 Max 20x 账号池,吞吐与效果最佳,推荐日常开发和重度场景使用。',
    models: ['claude-opus-4-6 (1M)', 'claude-opus-4-5', 'claude-sonnet-4-6', 'claude-sonnet-4-5', 'claude-haiku-4-5'],
    features: ['支持满血 thinking', '200k 上下文', '1M 上下文', '支持 WebSearch'],
    icon: ICON_MAX,
    highlight: true
  },
  {
    name: 'CC-Antigravity',
    badge: 'Claude',
    tagline: 'Balanced price & performance',
    rate: '1',
    credit: '1.00',
    desc: '反向接入通道,性能与价格均衡,适合对成本更敏感的工作负载。',
    models: ['claude-opus-4-6 (1M)', 'claude-sonnet-4-6 (1M)', 'claude-opus-4-5', 'claude-haiku-4-5'],
    features: ['支持 thinking', '200k 上下文', '支持 WebSearch'],
    icon: ICON_ORBIT
  },
  {
    name: 'OpenAI',
    badge: 'OpenAI',
    tagline: 'GPT-5 & Codex full access',
    rate: '1',
    credit: '1.00',
    desc: '官方 OpenAI 接入,覆盖 GPT-5 主力系列与 Codex 编程模型,适合从 Claude 切换或混用的工作流。',
    models: ['gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex', 'gpt-5.2', 'gpt-5.1-codex-max'],
    features: ['兼容 OpenAI SDK', '支持 Responses API', '支持 Codex', '支持流式'],
    icon: ICON_OPENAI
  }
]

const comparisonRows = [
  { feature: 'home.comparison.items.pricing.feature', official: 'home.comparison.items.pricing.official', us: 'home.comparison.items.pricing.us' },
  { feature: 'home.comparison.items.models.feature', official: 'home.comparison.items.models.official', us: 'home.comparison.items.models.us' },
  { feature: 'home.comparison.items.management.feature', official: 'home.comparison.items.management.official', us: 'home.comparison.items.management.us' },
  { feature: 'home.comparison.items.stability.feature', official: 'home.comparison.items.stability.official', us: 'home.comparison.items.stability.us' },
  { feature: 'home.comparison.items.control.feature', official: 'home.comparison.items.control.official', us: 'home.comparison.items.control.us' }
] as const

const faqItems = [
  { qKey: 'home.faq.items.buy.q', aKey: 'home.faq.items.buy.a' },
  { qKey: 'home.faq.items.activate.q', aKey: 'home.faq.items.activate.a' },
  { qKey: 'home.faq.items.expiry.q', aKey: 'home.faq.items.expiry.a' },
  { qKey: 'home.faq.items.models.q', aKey: 'home.faq.items.models.a' },
  { qKey: 'home.faq.items.refund.q', aKey: 'home.faq.items.refund.a' },
  { qKey: 'home.faq.items.support.q', aKey: 'home.faq.items.support.a' }
] as const
const openFaq = ref<number | null>(3)
function toggleFaq(idx: number) {
  openFaq.value = openFaq.value === idx ? null : idx
}

// ============================================================================
// Per-section list resolvers — admin config wins, otherwise use static defaults
// ============================================================================

type WhyChooseRow = { icon: string; title: string; desc: string }
const whyChooseList = computed<WhyChooseRow[]>(() => {
  const cfg = homeConfig.value?.whyChoose?.items
  if (Array.isArray(cfg) && cfg.length > 0) {
    return cfg.map((item) => ({
      icon: typeof item?.icon === 'string' && item.icon.trim() ? item.icon : 'sparkles',
      title: localized(item?.title, loc.value, ''),
      desc: localized(item?.desc, loc.value, '')
    }))
  }
  return whyChooseItems.map((item) => ({
    icon: item.icon,
    title: t(item.titleKey),
    desc: t(item.descKey)
  }))
})

const ICON_MAP: Record<string, string> = {
  bedrock: ICON_BEDROCK,
  max: ICON_MAX,
  orbit: ICON_ORBIT,
  openai: ICON_OPENAI
}

type ChannelRow = {
  name: string; badge: string; tagline: string
  rate: string; credit: string; desc: string
  models: string[]; features: string[]
  icon: string; highlight: boolean
}
const channelsList = computed<ChannelRow[]>(() => {
  const cfg = homeConfig.value?.channels?.items
  if (Array.isArray(cfg) && cfg.length > 0) {
    return cfg.map((item, i) => {
      const fallback = channels[i] ?? channels[0]
      const iconKey = (item?.icon || '').toLowerCase()
      return {
        name: item?.name || fallback?.name || '',
        badge: item?.badge || fallback?.badge || '',
        tagline: localized(item?.tagline, loc.value, fallback?.tagline ?? ''),
        rate: item?.rate || fallback?.rate || '',
        credit: item?.credit || fallback?.credit || '',
        desc: localized(item?.desc, loc.value, fallback?.desc ?? ''),
        models: Array.isArray(item?.models) && item.models.length > 0 ? item.models : (fallback?.models ?? []),
        features: Array.isArray(item?.features) && item.features.length > 0
          ? item.features.map((f) => localized(f, loc.value, ''))
          : (fallback?.features ?? []),
        icon: ICON_MAP[iconKey] ?? fallback?.icon ?? ICON_BEDROCK,
        highlight: typeof item?.highlight === 'boolean' ? item.highlight : Boolean(fallback?.highlight)
      }
    })
  }
  return channels.map((c) => ({
    name: c.name, badge: c.badge, tagline: c.tagline,
    rate: c.rate, credit: c.credit, desc: c.desc,
    models: c.models, features: c.features,
    icon: c.icon, highlight: Boolean(c.highlight)
  }))
})

type ComparisonRow = { feature: string; official: string; us: string }
const comparisonList = computed<ComparisonRow[]>(() => {
  const cfg = homeConfig.value?.comparison?.items
  if (Array.isArray(cfg) && cfg.length > 0) {
    return cfg.map((item) => ({
      feature: localized(item?.feature, loc.value, ''),
      official: localized(item?.official, loc.value, ''),
      us: localized(item?.us, loc.value, '')
    }))
  }
  return comparisonRows.map((row) => ({
    feature: t(row.feature),
    official: t(row.official),
    us: t(row.us)
  }))
})

type FaqRow = { q: string; a: string }
const faqList = computed<FaqRow[]>(() => {
  const cfg = homeConfig.value?.faq?.items
  if (Array.isArray(cfg) && cfg.length > 0) {
    return cfg.map((item) => ({
      q: localized(item?.q, loc.value, ''),
      a: localized(item?.a, loc.value, '')
    }))
  }
  return faqItems.map((item) => ({
    q: t(item.qKey),
    a: t(item.aKey)
  }))
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  document.addEventListener('click', handleLocaleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleLocaleClickOutside)
})
</script>

<style scoped>
.home-editorial {
  font-feature-settings: 'ss01', 'kern';
}

/* Sticky header offset for in-page anchor scrolling */
.home-editorial :deep(section[id]) {
  scroll-margin-top: 88px;
}

.font-display {
  font-family:
    'Iowan Old Style', 'Charter', 'Source Serif Pro', 'Source Serif 4', 'Tiempos Text',
    Georgia, 'Times New Roman',
    'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Noto Sans SC',
    sans-serif;
  font-weight: 400;
  letter-spacing: -0.005em;
}
/* CJK characters should use the lighter sans fallback with tighter leading
   — Chinese glyphs are visually denser than Latin. */
:lang(zh) .font-display,
:lang(zh-CN) .font-display,
:lang(zh-Hans) .font-display {
  font-weight: 300;
  letter-spacing: 0;
}

.eyebrow {
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  color: #CC785C;
}

.header-tools {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0 2px;
}
.header-tools-sep {
  display: block;
  width: 1px;
  height: 14px;
  background: rgba(26, 26, 26, 0.14);
}
.dark .header-tools-sep {
  background: rgba(245, 244, 238, 0.14);
}

.locale-dd {
  position: relative;
}
.locale-dd-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 6px;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  letter-spacing: 0.08em;
  color: rgba(26, 26, 26, 0.65);
  transition: color 150ms ease;
  cursor: pointer;
}
.locale-dd-trigger:hover {
  color: #1A1A1A;
}
.dark .locale-dd-trigger {
  color: rgba(245, 244, 238, 0.65);
}
.dark .locale-dd-trigger:hover {
  color: #F5F4EE;
}
.locale-dd-chevron {
  width: 9px;
  height: 9px;
  opacity: 0.7;
  transition: transform 180ms ease;
}
.locale-dd-chevron.is-open {
  transform: rotate(180deg);
}

.locale-dd-panel {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  min-width: 152px;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid rgba(26, 26, 26, 0.1);
  background: #FBFAF4;
  box-shadow:
    0 24px 48px -24px rgba(26, 26, 26, 0.22),
    0 4px 12px -6px rgba(26, 26, 26, 0.08);
  z-index: 60;
}
.dark .locale-dd-panel {
  background: #17150F;
  border-color: rgba(245, 244, 238, 0.1);
  box-shadow:
    0 24px 48px -24px rgba(0, 0, 0, 0.6),
    0 4px 12px -6px rgba(0, 0, 0, 0.4);
}

.locale-dd-option {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 10px;
  border-radius: 6px;
  font-size: 13px;
  color: rgba(26, 26, 26, 0.78);
  transition: background-color 120ms ease, color 120ms ease;
  cursor: pointer;
  text-align: left;
}
.locale-dd-option:hover {
  background: rgba(26, 26, 26, 0.04);
  color: #1A1A1A;
}
.locale-dd-option.is-active {
  color: #1A1A1A;
}
.locale-dd-option.is-active .locale-dd-code {
  color: #CC785C;
}
.dark .locale-dd-option {
  color: rgba(245, 244, 238, 0.78);
}
.dark .locale-dd-option:hover {
  background: rgba(245, 244, 238, 0.06);
  color: #F5F4EE;
}
.dark .locale-dd-option.is-active {
  color: #F5F4EE;
}
.locale-dd-code {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  letter-spacing: 0.1em;
  color: rgba(26, 26, 26, 0.45);
  min-width: 20px;
}
.dark .locale-dd-code {
  color: rgba(245, 244, 238, 0.45);
}
.locale-dd-name {
  flex: 1;
}
.locale-dd-check {
  width: 13px;
  height: 13px;
  color: #CC785C;
}

.locale-pop-enter-active,
.locale-pop-leave-active {
  transition: opacity 160ms ease, transform 160ms ease;
}
.locale-pop-enter-from,
.locale-pop-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.theme-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: rgba(26, 26, 26, 0.5);
  transition: color 150ms ease;
  cursor: pointer;
}
.theme-toggle:hover {
  color: rgba(26, 26, 26, 0.85);
}
.theme-toggle svg {
  width: 14px;
  height: 14px;
}
.dark .theme-toggle {
  color: rgba(245, 244, 238, 0.5);
}
.dark .theme-toggle:hover {
  color: rgba(245, 244, 238, 0.85);
}

.nav-link {
  display: inline-flex;
  align-items: center;
  font-size: 14px;
  font-weight: 500;
  color: rgba(26, 26, 26, 0.82);
  transition: color 150ms ease;
}
.nav-link:hover {
  color: #1A1A1A;
}
.dark .nav-link {
  color: rgba(245, 244, 238, 0.7);
}
.dark .nav-link:hover {
  color: #F5F4EE;
}

.ce-btn-primary {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  border-radius: 9999px;
  background-color: #1A1A1A;
  padding: 0.6rem 1.15rem;
  font-size: 13px;
  font-weight: 500;
  color: #F5F4EE;
  transition: background-color 150ms ease;
}
.ce-btn-primary:hover {
  background-color: #2A2A2A;
}
.dark .ce-btn-primary {
  background-color: #F5F4EE;
  color: #1A1A1A;
}
.dark .ce-btn-primary:hover {
  background-color: #ffffff;
}

.ce-btn-outline-sm {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  border-radius: 9999px;
  border: 1px solid rgba(26, 26, 26, 0.22);
  background-color: transparent;
  padding: 0.55rem 1.1rem;
  font-size: 13px;
  font-weight: 500;
  color: #1A1A1A;
  transition: border-color 150ms ease, background-color 150ms ease, color 150ms ease;
}
.ce-btn-outline-sm:hover {
  border-color: #1A1A1A;
  background-color: rgba(26, 26, 26, 0.035);
}
.dark .ce-btn-outline-sm {
  border-color: rgba(245, 244, 238, 0.25);
  color: #F5F4EE;
}
.dark .ce-btn-outline-sm:hover {
  border-color: #F5F4EE;
  background-color: rgba(245, 244, 238, 0.05);
}

.ce-btn-lg {
  padding: 0.95rem 1.8rem;
  font-size: 14px;
}

.ce-btn-accent {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  border-radius: 9999px;
  background-color: #CC785C;
  padding: 0.75rem 1.5rem;
  font-size: 13px;
  font-weight: 500;
  color: #FDFAF5;
  transition: background-color 150ms ease;
}
.ce-btn-accent:hover {
  background-color: #B86647;
}

.ce-btn-outline {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  border-radius: 9999px;
  border: 1px solid #1A1A1A;
  padding: 0.75rem 1.5rem;
  font-size: 13px;
  font-weight: 500;
  color: #1A1A1A;
  transition: all 150ms ease;
}
.ce-btn-outline:hover {
  background-color: #1A1A1A;
  color: #F5F4EE;
}
.dark .ce-btn-outline {
  border-color: #F5F4EE;
  color: #F5F4EE;
}
.dark .ce-btn-outline:hover {
  background-color: #F5F4EE;
  color: #1A1A1A;
}

.ce-btn-link {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 14px;
  font-weight: 500;
  color: #1A1A1A;
  text-decoration: underline;
  text-underline-offset: 6px;
  text-decoration-color: rgba(26, 26, 26, 0.22);
  transition: text-decoration-color 150ms ease;
}
.ce-btn-link:hover {
  text-decoration-color: #1A1A1A;
}
.dark .ce-btn-link {
  color: #F5F4EE;
  text-decoration-color: rgba(245, 244, 238, 0.22);
}
.dark .ce-btn-link:hover {
  text-decoration-color: #F5F4EE;
}

.channel-card {
  display: flex;
  flex-direction: column;
  padding: 36px 32px 32px;
  border-radius: 20px;
  border: 1px solid rgba(26, 26, 26, 0.1);
  background: #FBFAF4;
  transition: border-color 200ms ease, transform 200ms ease, box-shadow 200ms ease;
}
.channel-card:hover {
  border-color: rgba(26, 26, 26, 0.22);
  transform: translateY(-2px);
  box-shadow: 0 18px 40px -28px rgba(26, 26, 26, 0.22);
}
.dark .channel-card {
  background: #15130E;
  border-color: rgba(245, 244, 238, 0.1);
}
.channel-card-highlight {
  border-color: rgba(204, 120, 92, 0.4);
  background: #FBF7F1;
}
.channel-card-highlight:hover {
  border-color: rgba(204, 120, 92, 0.55);
}
.dark .channel-card-highlight {
  background: #181410;
  border-color: rgba(204, 120, 92, 0.35);
}

.channel-icon {
  width: 44px;
  height: 44px;
  color: #1A1A1A;
  margin-bottom: 28px;
}
.channel-icon :deep(svg) {
  width: 100%;
  height: 100%;
}
.dark .channel-icon {
  color: #F5F4EE;
}
.channel-card-highlight .channel-icon {
  color: #CC785C;
}

.channel-name {
  font-size: 34px;
  line-height: 1.05;
  letter-spacing: -0.015em;
  color: #1A1A1A;
}
.dark .channel-name {
  color: #F5F4EE;
}
.channel-tagline {
  margin-top: 6px;
  font-size: 14px;
  color: rgba(26, 26, 26, 0.55);
}
.dark .channel-tagline {
  color: rgba(245, 244, 238, 0.55);
}

.channel-price {
  margin-top: 26px;
}
.channel-price-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.channel-price-row span:first-child {
  font-size: 36px;
  line-height: 1;
  color: #1A1A1A;
}
.dark .channel-price-row span:first-child {
  color: #F5F4EE;
}
.channel-price-label {
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: rgba(26, 26, 26, 0.45);
}
.dark .channel-price-label {
  color: rgba(245, 244, 238, 0.45);
}
.channel-credit {
  margin-top: 6px;
  font-size: 12px;
  color: rgba(26, 26, 26, 0.5);
}
.dark .channel-credit {
  color: rgba(245, 244, 238, 0.5);
}

.channel-cta {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 26px;
  padding: 13px 0;
  border-radius: 9999px;
  background-color: #1A1A1A;
  color: #F5F4EE;
  font-size: 14px;
  font-weight: 500;
  transition: background-color 150ms ease;
}
.channel-cta:hover {
  background-color: #2A2A2A;
}
.dark .channel-cta {
  background-color: #F5F4EE;
  color: #1A1A1A;
}
.dark .channel-cta:hover {
  background-color: #ffffff;
}
.channel-card-highlight .channel-cta {
  background-color: #CC785C;
  color: #FDFAF5;
}
.channel-card-highlight .channel-cta:hover {
  background-color: #B86647;
}
.channel-fineprint {
  margin-top: 10px;
  text-align: center;
  font-size: 11px;
  color: rgba(26, 26, 26, 0.45);
}
.dark .channel-fineprint {
  color: rgba(245, 244, 238, 0.45);
}

.channel-divider {
  margin: 26px 0 22px;
  height: 1px;
  background: rgba(26, 26, 26, 0.08);
}
.dark .channel-divider {
  background: rgba(245, 244, 238, 0.08);
}

.channel-section-title {
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: rgba(26, 26, 26, 0.5);
}
.dark .channel-section-title {
  color: rgba(245, 244, 238, 0.5);
}

.channel-list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.channel-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: rgba(26, 26, 26, 0.82);
  line-height: 1.45;
}
.dark .channel-list li {
  color: rgba(245, 244, 238, 0.82);
}
.channel-list .check {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  color: #CC785C;
}
.channel-list-more {
  font-size: 12px;
  color: rgba(26, 26, 26, 0.45);
  padding-left: 24px;
}
.dark .channel-list-more {
  color: rgba(245, 244, 238, 0.45);
}
.channel-list .font-mono {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}

.code-card {
  margin: 0;
  border-radius: 14px;
  border: 1px solid rgba(26, 26, 26, 0.1);
  background: #FBFAF4;
  box-shadow: 0 24px 60px -38px rgba(26, 26, 26, 0.22);
  overflow: hidden;
}
.dark .code-card {
  background: #17150F;
  border-color: rgba(245, 244, 238, 0.1);
}
.code-card-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 18px;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: rgba(26, 26, 26, 0.5);
  letter-spacing: 0.02em;
  border-bottom: 1px solid rgba(26, 26, 26, 0.06);
}
.dark .code-card-label {
  color: rgba(245, 244, 238, 0.5);
  border-bottom-color: rgba(245, 244, 238, 0.06);
}
.code-card-lang {
  text-transform: uppercase;
  letter-spacing: 0.14em;
  font-size: 10px;
}
.code-card-body {
  margin: 0;
  padding: 18px 22px 22px;
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace;
  font-size: 13px;
  line-height: 1.65;
  color: rgba(26, 26, 26, 0.88);
  overflow-x: auto;
  white-space: pre;
  -webkit-font-smoothing: antialiased;
}
.dark .code-card-body {
  color: rgba(245, 244, 238, 0.88);
}
.code-card-body code { font-family: inherit; }
.code-card-body .ln { display: block; }
.code-card-body .kw { color: #CC785C; }
.code-card-body .str { color: #8A6B56; }
.dark .code-card-body .str { color: #D4A088; }

.footer-link {
  font-size: 13px;
  color: rgba(26, 26, 26, 0.8);
  transition: color 150ms ease;
}
.footer-link:hover {
  color: #CC785C;
}
.dark .footer-link {
  color: rgba(245, 244, 238, 0.8);
}
.dark .footer-link:hover {
  color: #CC785C;
}
</style>
