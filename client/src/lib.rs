use cpal::traits::DeviceTrait;

pub fn find_loopback_device(devices: &[cpal::Device]) -> Option<&cpal::Device> {
    let loopback_names = ["stereo mix", "loopback", "blackhole", "soundflower"];
    for device in devices {
        if let Ok(name) = device.name() {
            let name_lower = name.to_lowercase();
            if loopback_names.iter().any(|&lb| name_lower.contains(lb)) {
                return Some(device);
            }
        }
    }
    None
}

pub fn select_device(
    devices: &[cpal::Device],
    device_index: Option<usize>,
    device_name: Option<&str>,
) -> Option<&cpal::Device> {
    if let Some(index) = device_index {
        devices.get(index).filter(|d| {
            d.supported_input_configs().map(|c| c.count() > 0).unwrap_or(false)
        })
    } else if let Some(name) = device_name {
        devices.iter().find(|d| {
            d.name().map(|n| n == name).unwrap_or(false) &&
            d.supported_input_configs().map(|c| c.count() > 0).unwrap_or(false)
        })
    } else {
        find_loopback_device(devices).or_else(|| {
            devices.iter().find(|d| {
                d.supported_input_configs().map(|c| c.count() > 0).unwrap_or(false)
            })
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Mock device for testing
    struct MockDevice {
        name: String,
        has_input: bool,
    }

    impl MockDevice {
        fn new(name: &str, has_input: bool) -> Self {
            Self {
                name: name.to_string(),
                has_input,
            }
        }
    }

    impl DeviceTrait for MockDevice {
        type SupportedInputConfigs = std::iter::Empty<cpal::SupportedStreamConfigRange>;
        type SupportedOutputConfigs = std::iter::Empty<cpal::SupportedStreamConfigRange>;
        type Stream = cpal::Stream;

        fn name(&self) -> Result<String, cpal::DeviceNameError> {
            Ok(self.name.clone())
        }

        fn supported_input_configs(&self) -> Result<Self::SupportedInputConfigs, cpal::SupportedStreamConfigsError> {
            if self.has_input {
                Ok(std::iter::empty())
            } else {
                Err(cpal::SupportedStreamConfigsError::DeviceNotAvailable)
            }
        }

        fn supported_output_configs(&self) -> Result<Self::SupportedOutputConfigs, cpal::SupportedStreamConfigsError> {
            Ok(std::iter::empty())
        }

        fn default_input_config(&self) -> Result<cpal::SupportedStreamConfig, cpal::DefaultStreamConfigError> {
            unimplemented!()
        }

        fn default_output_config(&self) -> Result<cpal::SupportedStreamConfig, cpal::DefaultStreamConfigError> {
            unimplemented!()
        }

        fn build_input_stream_raw<D, E>(
            &self,
            _config: &cpal::StreamConfig,
            _data_callback: D,
            _error_callback: E,
            _timeout: Option<std::time::Duration>,
        ) -> Result<Self::Stream, cpal::BuildStreamError>
        where
            D: FnMut(&cpal::Data, &cpal::InputCallbackInfo) + Send + 'static,
            E: FnMut(cpal::StreamError) + Send + 'static,
        {
            unimplemented!()
        }

        fn build_output_stream_raw<D, E>(
            &self,
            _config: &cpal::StreamConfig,
            _data_callback: D,
            _error_callback: E,
            _timeout: Option<std::time::Duration>,
        ) -> Result<Self::Stream, cpal::BuildStreamError>
        where
            D: FnMut(&mut cpal::Data, &cpal::OutputCallbackInfo) + Send + 'static,
            E: FnMut(cpal::StreamError) + Send + 'static,
        {
            unimplemented!()
        }

        fn build_input_stream<D, E>(
            &self,
            _config: &cpal::StreamConfig,
            _data_callback: D,
            _error_callback: E,
        ) -> Result<Self::Stream, cpal::BuildStreamError>
        where
            D: FnMut(&cpal::Data, &cpal::InputCallbackInfo) + Send + 'static,
            E: FnMut(cpal::StreamError) + Send + 'static,
        {
            unimplemented!()
        }

        fn build_output_stream<D, E>(
            &self,
            _config: &cpal::StreamConfig,
            _data_callback: D,
            _error_callback: E,
        ) -> Result<Self::Stream, cpal::BuildStreamError>
        where
            D: FnMut(&mut cpal::Data, &cpal::OutputCallbackInfo) + Send + 'static,
            E: FnMut(cpal::StreamError) + Send + 'static,
        {
            unimplemented!()
        }
    }

    #[test]
    fn test_find_loopback_device() {
        let devices = vec![
            MockDevice::new("Microphone", true),
            MockDevice::new("Stereo Mix", true),
            MockDevice::new("Speakers", false),
        ];

        let result = find_loopback_device(&devices);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name().unwrap(), "Stereo Mix");
    }

    #[test]
    fn test_find_loopback_device_no_match() {
        let devices = vec![
            MockDevice::new("Microphone", true),
            MockDevice::new("Speakers", false),
        ];

        let result = find_loopback_device(&devices);
        assert!(result.is_none());
    }

    #[test]
    fn test_select_device_by_index() {
        let devices = vec![
            MockDevice::new("Device1", true),
            MockDevice::new("Device2", true),
        ];

        let result = select_device(&devices, Some(0), None);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name().unwrap(), "Device1");
    }

    #[test]
    fn test_select_device_by_name() {
        let devices = vec![
            MockDevice::new("Microphone", true),
            MockDevice::new("Stereo Mix", true),
        ];

        let result = select_device(&devices, None, Some("Stereo Mix"));
        assert!(result.is_some());
        assert_eq!(result.unwrap().name().unwrap(), "Stereo Mix");
    }

    #[test]
    fn test_select_device_default_loopback() {
        let devices = vec![
            MockDevice::new("Microphone", true),
            MockDevice::new("Stereo Mix", true),
        ];

        let result = select_device(&devices, None, None);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name().unwrap(), "Stereo Mix");
    }

    #[test]
    fn test_select_device_default_fallback() {
        let devices = vec![
            MockDevice::new("Microphone", true),
            MockDevice::new("Speakers", false),
        ];

        let result = select_device(&devices, None, None);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name().unwrap(), "Microphone");
    }
}