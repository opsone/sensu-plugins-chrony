#!/usr/bin/env ruby
# frozen_string_literal: true

require 'sensu-plugin/check/cli'

class CheckChrony < Sensu::Plugin::Check::CLI
  option :chronyc_cmd,
         description: 'Path to chronyc executable (default: /usr/bin/chronyc)',
         short: '-c <PATH>',
         long: '--chronyc-cmd <PATH>',
         default: '/usr/bin/chronyc'

  option :warn_offset,
         description: 'Warn if OFFSET exceeds current offset (ms)',
         long: '--warn-offset <OFFSET>',
         proc: proc(&:to_f),
         default: 50

  option :crit_offset,
         description: 'Critical if OFFSET exceeds current offset (ms)',
         long: '--crit-offset <OFFSET>',
         proc: proc(&:to_f),
         default: 100

  option :warn_stratum,
         description: 'Warn if STRATUM exceeds current stratum',
         long: '--warn-stratum <STRATUM>',
         proc: proc(&:to_i),
         default: 10

  option :crit_stratum,
         description: 'Critical if STRATUM exceeds current stratum',
         long: '--crit-stratum <STRATUM>',
         proc: proc(&:to_i),
         default: 16

  def run
    stratum = nil
    offset = nil
    status = nil

    `#{config[:chronyc_cmd]} tracking`.each_line do |line|
      case line.downcase
      when /^stratum\s*:\s*(\d+)$/
        stratum = Regexp.last_match(1).to_i
      when /^last offset\s*:\s*([\-\+]?[.\d]+)\s*seconds$/
        # convert from seconds to milliseconds
        offset = Regexp.last_match(1).to_f * 1000
      when /^leap status\s*:\s*(.*?)$/
        status = Regexp.last_match(1)
      end
    end

    if stratum
      msg = "NTP stratum is #{stratum}"

      if stratum >= config[:crit_stratum]
        msg += ", expected < #{config[:crit_stratum]}"
        critical msg
      elsif stratum >= config[:warn_stratum]
        msg += ", expected < #{config[:warn_stratum]}"
        warning msg
      end
    else
      unknown 'Failed to look up NTP stratum'
    end

    if offset
      msg = "NTP offset is #{offset.round(4)}ms"

      if offset >= config[:crit_offset] || offset <= -config[:crit_offset]
        msg += ", expected > -#{config[:crit_offset]} and < #{config[:crit_offset]}"
        critical msg
      elsif offset >= config[:warn_offset] || offset < -config[:warn_offset]
        msg += ", expected > -#{config[:warn_offset]} and < #{config[:warn_offset]}"
        warning msg
      else
        ok msg
      end
    else
      unknown 'Failed to look up NTP offset'
    end
  end
end
