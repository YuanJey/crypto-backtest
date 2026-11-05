/**
 * Strategy contract compatibility boundary. Every operation returns a new
 * definition; API revisions and editable drafts are never migrated in place.
 */
const StrategyContractAdapter = (() => {
  const clone = value => {
    if (Array.isArray(value)) return value.map(clone);
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, clone(item)]));
    }
    return value;
  };

  function definitionCopy(source) {
    if (!source || typeof source !== 'object' || Array.isArray(source)) {
      throw new Error('策略必须是 JSON 对象');
    }
    const def = clone(source);
    for (const key of ['account', 'position', 'risk', 'behavior', 'data']) {
      if (def[key] !== undefined &&
          (!def[key] || typeof def[key] !== 'object' || Array.isArray(def[key]))) {
        throw new Error(`${key} 必须是 JSON 对象`);
      }
    }
    for (const key of ['rules', 'indicators', 'exit_policies']) {
      if (def[key] !== undefined && !Array.isArray(def[key])) {
        throw new Error(`${key} 必须是数组`);
      }
      for (const item of def[key] || []) {
        if (!item || typeof item !== 'object' || Array.isArray(item)) {
          throw new Error(`${key} 中的配置必须是 JSON 对象`);
        }
      }
    }
    for (const rule of def.rules || []) {
      if (!rule.action || typeof rule.action !== 'object' || Array.isArray(rule.action)) {
        throw new Error(`规则 ${rule.id || ''} 缺少有效 action`);
      }
    }
    for (const policy of def.exit_policies || []) {
      for (const key of ['anchor', 'distance']) {
        if (policy[key] !== undefined &&
            (!policy[key] || typeof policy[key] !== 'object' || Array.isArray(policy[key]))) {
          throw new Error(`退出策略 ${policy.id || ''} 的 ${key} 必须是 JSON 对象`);
        }
      }
    }
    return def;
  }

  function migrateSizing(def) {
    const position = def.position || {};
    const behavior = def.behavior || {};
    const rules = def.rules || [];
    const hasLegacyOpen = rules.some(rule => rule.action.type === 'open');
    const allowAdditions = behavior.allow_pyramiding || position.allow_pyramiding;
    const maxAdditions = position.max_additions === undefined ? 0 : position.max_additions;
    for (const value of [position.allow_pyramiding, behavior.allow_pyramiding]) {
      if (value !== undefined && typeof value !== 'boolean') {
        throw new Error('allow_pyramiding 必须是布尔值');
      }
    }
    if (hasLegacyOpen) {
      if (!Number.isSafeInteger(maxAdditions) || maxAdditions < 0) {
        throw new Error('最大加仓次数必须是非负整数');
      }
      for (const field of ['initial_size_pct', 'add_size_pct', 'add_size_multiplier']) {
        const value = position[field] === undefined ? 0 : position[field];
        if (!Number.isFinite(value) || value < 0 ||
            (field !== 'add_size_multiplier' && value > 1)) {
          throw new Error(`${field} 无效`);
        }
      }
      if (position.add_size_pct > 0 && position.add_size_multiplier > 0) {
        throw new Error('固定比例与倍增配置不能同时启用');
      }
      if (maxAdditions > 0 && !allowAdditions) {
        throw new Error('加仓次数与加仓开关不一致');
      }
      if (maxAdditions === 0 && (position.add_size_pct > 0 || position.add_size_multiplier > 0)) {
        throw new Error('加仓配置缺少适用次数');
      }
      const ids = new Set(rules.map(rule => rule.id));
      const converted = [];
      for (const rule of rules) {
        if (rule.action.type !== 'open') {
          converted.push(rule);
          continue;
        }
        // Zero is the legacy inheritance sentinel, not a new allocation.
        const sizePct = rule.action.size_pct === undefined || rule.action.size_pct === 0
          ? (position.initial_size_pct ?? 0) : rule.action.size_pct;
        if (!rule.disabled && (!Number.isFinite(sizePct) || sizePct <= 0 || sizePct > 1)) {
          throw new Error(`规则 ${rule.id} 缺少有效的开仓比例`);
        }
        const sourceID = rule.source_rule_id || rule.id;
        converted.push({
          ...clone(rule), source_rule_id: sourceID,
          action: { ...rule.action, type: 'enter', size_pct: sizePct },
        });
        for (let index = 1; allowAdditions && index <= maxAdditions; index++) {
          const baseID = `${rule.id}__add_${index}`;
          let id = baseID;
          for (let suffix = 2; ids.has(id); suffix++) id = `${baseID}_${suffix}`;
          ids.add(id);
          const action = { type: 'add', side: rule.action.side, addition_index: index };
          if (rule.action.size_pct > 0 || position.add_size_pct > 0) {
            action.size_pct = rule.action.size_pct || position.add_size_pct;
          } else {
            action.size_multiplier = position.add_size_multiplier > 0
              ? Math.pow(position.add_size_multiplier, index) : 1;
            if (!Number.isFinite(action.size_multiplier) || action.size_multiplier <= 0) {
              throw new Error(`规则 ${rule.id} 第 ${index} 次加仓倍数超出范围`);
            }
          }
          converted.push({
            ...clone(rule), id, source_rule_id: sourceID,
            name: `${rule.name || rule.id} · 第 ${index} 次加仓`, action,
          });
        }
      }
      def.rules = converted;
    }
    for (const field of [
      'initial_size_pct', 'allow_pyramiding', 'max_additions',
      'add_size_pct', 'add_size_multiplier',
    ]) delete position[field];
    const migratedBehavior = def.behavior || (def.behavior = {});
    if (migratedBehavior.allow_additions === undefined) {
      migratedBehavior.allow_additions = (def.rules || [])
        .some(rule => rule.action.type === 'add' && !rule.disabled);
    }
    migratedBehavior.allow_pyramiding = false;
    return def;
  }

  function migrateLeverage(def) {
    const account = def.account || (def.account = {});
    const position = def.position || (def.position = {});
    const risk = def.risk || (def.risk = {});
    if (account.exchange_leverage === undefined) account.exchange_leverage = 1;
    if (position.position_leverage === undefined) {
      position.position_leverage = position.target_leverage === undefined ? 1 : position.target_leverage;
    }
    for (const field of ['long_capital_decay_power', 'short_capital_decay_power']) {
      if (position[field] === undefined) position[field] = 0.5;
    }
    delete position.target_leverage;
    delete position.max_position_pct;
    delete risk.max_symbol_equity_pct;
    delete risk.max_total_exposure_pct;
    return def;
  }

  return Object.freeze({
    clone,
    migrateRuleSizing: source => migrateSizing(definitionCopy(source)),
    migrateLeverageConfig: source => migrateLeverage(definitionCopy(source)),
    prepareDraft: source => migrateLeverage(migrateSizing(definitionCopy(source))),
    prepareSave: source => migrateLeverage(migrateSizing(definitionCopy(source))),
  });
})();

window.StrategyContractAdapter = StrategyContractAdapter;
